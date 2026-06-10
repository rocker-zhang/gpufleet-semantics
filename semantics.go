// Package semantics is the adoption-wedge library for gpufleet: it maps
// devices to jobs and computes deterministic cost/efficiency attribution
// (MFU, tensor-active fraction, straggler ratio, $ cost) from already-collected
// metrics. It performs NO collection and talks to NO GPU — inputs are plain
// numbers supplied by the agent. Everything here is pure, deterministic math.
package semantics

import (
	"errors"
	"sort"
)

// Device identifies a single physical accelerator within a fleet.
type Device struct {
	UUID  string // stable hardware UUID (e.g. GPU-xxxxxxxx)
	Node  string // host the device lives on
	Model string // e.g. "A10", "GB10"
}

// Job identifies a unit of work that may span multiple devices.
type Job struct {
	ID    string
	Owner string
}

// DeviceJob is a single resolved (device -> job) ownership edge for a window.
type DeviceJob struct {
	Device Device
	Job    Job
}

// DeviceSpec captures the fixed performance characteristics needed for MFU.
// PeakFLOPS is the device's peak dense throughput in FLOP/s for the dtype the
// job actually ran (caller picks the right peak, e.g. BF16 tensor-core peak).
type DeviceSpec struct {
	PeakFLOPS   float64 // > 0
	CostPerHour float64 // USD/hour for $ attribution; may be 0 if unknown
}

// DeviceSample is a normalized measurement window for one device.
type DeviceSample struct {
	Device           Device
	WindowSeconds    float64 // > 0
	AchievedFLOPs    float64 // total floating-point ops done in the window
	TensorActiveSecs float64 // seconds tensor pipes were active (0..WindowSeconds)
}

// DeviceEfficiency is the per-device attribution result.
type DeviceEfficiency struct {
	Device       Device
	MFU          float64 // achieved FLOP/s / peak FLOP/s, clamped to [0,1]
	TensorActive float64 // tensor-active fraction, clamped to [0,1]
	CostUSD      float64 // CostPerHour * window hours
}

// JobEfficiency aggregates device attribution up to the job level, including
// the straggler ratio: how far the slowest device lags the fastest by MFU.
type JobEfficiency struct {
	Job            Job
	Devices        []DeviceEfficiency
	MeanMFU        float64
	StragglerRatio float64 // (max-min)/max of per-device MFU; 0 means balanced
	CostUSD        float64
}

var (
	// ErrBadWindow is returned for a non-positive measurement window.
	ErrBadWindow = errors.New("semantics: window seconds must be > 0")
	// ErrBadPeak is returned for a non-positive peak FLOP/s spec.
	ErrBadPeak = errors.New("semantics: peak FLOPS must be > 0")
)

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// DeviceEff computes MFU, tensor-active fraction, and $ cost for one device.
func DeviceEff(s DeviceSample, spec DeviceSpec) (DeviceEfficiency, error) {
	if s.WindowSeconds <= 0 {
		return DeviceEfficiency{}, ErrBadWindow
	}
	if spec.PeakFLOPS <= 0 {
		return DeviceEfficiency{}, ErrBadPeak
	}
	achievedRate := s.AchievedFLOPs / s.WindowSeconds
	hours := s.WindowSeconds / 3600.0
	return DeviceEfficiency{
		Device:       s.Device,
		MFU:          clamp01(achievedRate / spec.PeakFLOPS),
		TensorActive: clamp01(s.TensorActiveSecs / s.WindowSeconds),
		CostUSD:      spec.CostPerHour * hours,
	}, nil
}

// JobEff aggregates per-device efficiency for a single job. Devices are sorted
// by UUID so the output is deterministic regardless of input ordering.
func JobEff(job Job, devs []DeviceEfficiency) JobEfficiency {
	out := JobEfficiency{Job: job, Devices: append([]DeviceEfficiency(nil), devs...)}
	sort.Slice(out.Devices, func(i, j int) bool {
		return out.Devices[i].Device.UUID < out.Devices[j].Device.UUID
	})
	if len(out.Devices) == 0 {
		return out
	}
	var sum, minMFU, maxMFU float64
	minMFU = out.Devices[0].MFU
	maxMFU = out.Devices[0].MFU
	for _, d := range out.Devices {
		sum += d.MFU
		out.CostUSD += d.CostUSD
		if d.MFU < minMFU {
			minMFU = d.MFU
		}
		if d.MFU > maxMFU {
			maxMFU = d.MFU
		}
	}
	out.MeanMFU = sum / float64(len(out.Devices))
	if maxMFU > 0 {
		out.StragglerRatio = (maxMFU - minMFU) / maxMFU
	}
	return out
}
