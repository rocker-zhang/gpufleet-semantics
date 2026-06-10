package semantics

import (
	"math"
	"testing"
)

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestDeviceEffFixedInputs(t *testing.T) {
	// 100s window, 1e15 achieved FLOPs -> 1e13 FLOP/s achieved.
	// Peak 5e13 FLOP/s -> MFU = 0.2. Tensor active 40s of 100s -> 0.4.
	// Cost 3.6 USD/hr * (100/3600) hr = 0.1 USD.
	got, err := DeviceEff(DeviceSample{
		Device:           Device{UUID: "GPU-1", Node: "n1", Model: "A10"},
		WindowSeconds:    100,
		AchievedFLOPs:    1e15,
		TensorActiveSecs: 40,
	}, DeviceSpec{PeakFLOPS: 5e13, CostPerHour: 3.6})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !almost(got.MFU, 0.2) {
		t.Errorf("MFU = %v, want 0.2", got.MFU)
	}
	if !almost(got.TensorActive, 0.4) {
		t.Errorf("TensorActive = %v, want 0.4", got.TensorActive)
	}
	if !almost(got.CostUSD, 0.1) {
		t.Errorf("CostUSD = %v, want 0.1", got.CostUSD)
	}
}

func TestDeviceEffClampAndErrors(t *testing.T) {
	if _, err := DeviceEff(DeviceSample{WindowSeconds: 0}, DeviceSpec{PeakFLOPS: 1}); err != ErrBadWindow {
		t.Errorf("want ErrBadWindow, got %v", err)
	}
	if _, err := DeviceEff(DeviceSample{WindowSeconds: 1}, DeviceSpec{PeakFLOPS: 0}); err != ErrBadPeak {
		t.Errorf("want ErrBadPeak, got %v", err)
	}
	// Over-peak achieved rate must clamp to 1.0, never exceed.
	got, _ := DeviceEff(DeviceSample{WindowSeconds: 1, AchievedFLOPs: 1e18, TensorActiveSecs: 5},
		DeviceSpec{PeakFLOPS: 1e12})
	if got.MFU != 1.0 {
		t.Errorf("MFU clamp = %v, want 1.0", got.MFU)
	}
	if got.TensorActive != 1.0 {
		t.Errorf("TensorActive clamp = %v, want 1.0", got.TensorActive)
	}
}

func TestJobEffStraggler(t *testing.T) {
	job := Job{ID: "job-7", Owner: "team-a"}
	devs := []DeviceEfficiency{
		{Device: Device{UUID: "GPU-b"}, MFU: 0.4, CostUSD: 0.1},
		{Device: Device{UUID: "GPU-a"}, MFU: 0.8, CostUSD: 0.1},
	}
	je := JobEff(job, devs)
	// Sorted by UUID: GPU-a first.
	if je.Devices[0].Device.UUID != "GPU-a" {
		t.Errorf("devices not sorted deterministically: %s", je.Devices[0].Device.UUID)
	}
	if !almost(je.MeanMFU, 0.6) {
		t.Errorf("MeanMFU = %v, want 0.6", je.MeanMFU)
	}
	// (0.8-0.4)/0.8 = 0.5
	if !almost(je.StragglerRatio, 0.5) {
		t.Errorf("StragglerRatio = %v, want 0.5", je.StragglerRatio)
	}
	if !almost(je.CostUSD, 0.2) {
		t.Errorf("CostUSD = %v, want 0.2", je.CostUSD)
	}
}

func TestJobEffEmpty(t *testing.T) {
	je := JobEff(Job{ID: "empty"}, nil)
	if je.MeanMFU != 0 || je.StragglerRatio != 0 || je.CostUSD != 0 {
		t.Errorf("empty job should be zero-valued, got %+v", je)
	}
}
