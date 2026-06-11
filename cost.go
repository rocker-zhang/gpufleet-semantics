package semantics

import (
	"fmt"
	"sort"
)

// ----------------------------------------------------------------------------
// Standalone deterministic cost wedge (TASK-0015).
//
// The cost/efficiency attribution here is computable WITHOUT any fault, RCA, or
// gate decision: every input window yields a CostImpact, including healthy and
// ABSTAIN windows. Wasted spend is attributed deterministically from idle and
// low-utilization time, derived only from the device's measured MFU /
// tensor-active fraction and its cost rate. No signals, no playbooks, no gate.
//
// This is the "money before RCA" path: cost is a first-class output, never a
// by-product of a fault firing.
// ----------------------------------------------------------------------------

// CostPolicy parameterizes the standalone cost wedge. Defaults (DefaultCostPolicy)
// are deterministic and conservative; callers may override to match a fleet's
// billing model. All thresholds are fractions in [0,1].
type CostPolicy struct {
	// MFUFloor is the MFU below which a window is considered LOW utilization.
	// A device running at or above this MFU wastes no "low-util" dollars.
	MFUFloor float64
	// TensorActiveFloor is the tensor-active fraction below which the device is
	// considered underutilized for the LOW_UTILIZATION signature. Used together
	// with MFUFloor as the >=2-condition rule (see LowUtilization).
	TensorActiveFloor float64
}

// DefaultCostPolicy is the deterministic default: a window achieving < 20% MFU
// is treated as wasting the shortfall below the floor, and < 20% tensor-active
// is the corroborating low-utilization condition. These are billing-model
// defaults, NOT a gate threshold — the gate lives in rca/closed.
func DefaultCostPolicy() CostPolicy {
	return CostPolicy{MFUFloor: 0.20, TensorActiveFloor: 0.20}
}

func (p CostPolicy) withDefaults() CostPolicy {
	if p.MFUFloor <= 0 {
		p.MFUFloor = 0.20
	}
	if p.TensorActiveFloor <= 0 {
		p.TensorActiveFloor = 0.20
	}
	return p
}

// CostWedge is the standalone per-device cost attribution for one window. It is
// always produced (TASK-0015): a healthy device simply has zero WastedUSD.
type CostWedge struct {
	Device DeviceEfficiency
	// IdleFraction is the fraction of the window the device was NOT doing useful
	// work, approximated as 1-MFU (clamped to [0,1]). It is the deterministic
	// basis for wasted spend.
	IdleFraction float64
	// WastedUSD is the spend attributed to idle/low-utilization over the window:
	// CostUSD * IdleFraction. Zero when CostPerHour is unknown or MFU == 1.
	WastedUSD float64
	// LowUtilization reports whether this window meets the deterministic
	// LOW_UTILIZATION rule (MFU below floor AND tensor-active below floor). It is
	// informational here; the binding gate decision is rca/closed's.
	LowUtilization bool
	// Impact is the proto-shaped (mirror) CostImpact for this window, suitable
	// for placing on a Verdict regardless of fault_class.
	Impact CostImpact
}

// DeviceCostWedge computes the standalone cost wedge for one device window. It
// requires NO fault and NO RCA: idle/low-utilization dollars are attributed on
// every input. Returns the same ErrBadWindow/ErrBadPeak as DeviceEff.
func DeviceCostWedge(s DeviceSample, spec DeviceSpec, policy CostPolicy) (CostWedge, error) {
	eff, err := DeviceEff(s, spec)
	if err != nil {
		return CostWedge{}, err
	}
	return costWedgeFromEff(eff, spec, policy.withDefaults()), nil
}

// costWedgeFromEff is the pure attribution step shared by the per-device and
// per-job paths; eff is already validated/clamped.
func costWedgeFromEff(eff DeviceEfficiency, spec DeviceSpec, policy CostPolicy) CostWedge {
	idle := clamp01(1.0 - eff.MFU)
	priced := spec.CostPerHour > 0
	wasted := 0.0
	if priced {
		wasted = eff.CostUSD * idle
	}
	lowUtil := eff.MFU < policy.MFUFloor && eff.TensorActive < policy.TensorActiveFloor

	w := CostWedge{
		Device:         eff,
		IdleFraction:   idle,
		WastedUSD:      wasted,
		LowUtilization: lowUtil,
	}
	w.Impact = CostImpact{
		Computed:   priced,
		UsdWindow:  wasted,
		UsdPerHour: 0,
		Basis:      "",
	}
	if priced {
		w.Impact.UsdPerHour = spec.CostPerHour * idle
		w.Impact.Basis = fmt.Sprintf("%s idle %.0f%% x $%.2f/hr", eff.Device.UUID, idle*100, spec.CostPerHour)
	}
	return w
}

// JobCostWedge aggregates standalone cost wedges across a job's devices. Like
// JobEff it is deterministic: devices are sorted by UUID and the aggregate
// CostImpact sums the per-device wasted spend. Produced for every job, with no
// fault/RCA dependency. The per-device wedges are paired with already-computed
// DeviceEfficiency values (so callers reuse one DeviceEff pass) and their
// matching specs keyed by device UUID.
func JobCostWedge(job Job, devs []DeviceEfficiency, specs map[string]DeviceSpec, policy CostPolicy) JobCostImpact {
	policy = policy.withDefaults()
	sorted := append([]DeviceEfficiency(nil), devs...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Device.UUID < sorted[j].Device.UUID
	})

	out := JobCostImpact{Job: job, Wedges: make([]CostWedge, 0, len(sorted))}
	anyPriced := false
	var totalWasted, totalPerHour float64
	for _, eff := range sorted {
		w := costWedgeFromEff(eff, specs[eff.Device.UUID], policy)
		out.Wedges = append(out.Wedges, w)
		if w.Impact.Computed {
			anyPriced = true
			totalWasted += w.Impact.UsdWindow
			totalPerHour += w.Impact.UsdPerHour
		}
	}
	out.Impact = CostImpact{
		Computed:   anyPriced,
		UsdWindow:  totalWasted,
		UsdPerHour: totalPerHour,
	}
	if anyPriced {
		out.Impact.Basis = fmt.Sprintf("job %s: $%.4f wasted over window across %d device(s)",
			job.ID, totalWasted, len(out.Wedges))
	}
	return out
}

// JobCostImpact is the job-level standalone cost attribution: per-device wedges
// plus an aggregate proto-shaped CostImpact. Always produced (TASK-0015).
type JobCostImpact struct {
	Job    Job
	Wedges []CostWedge
	Impact CostImpact
}

// LowUtilizationSignal is a deterministic, fault-free description of the
// LOW_UTILIZATION condition for a device window, carrying the proto signature
// id (mirror) for the open<->closed shared registry. It encodes the >=2
// independent-condition rule (low MFU AND low tensor-active) as a structured,
// auditable fact — NOT a gate decision. rca/closed adjudicates; this only
// reports the deterministic condition and its supporting numbers.
type LowUtilizationSignal struct {
	Device DeviceEfficiency
	// Fired is true when the window meets the deterministic low-utilization rule.
	Fired bool
	// Signature is the mirrored gate-signature id. GateSignatureLowUtilization
	// when Fired, GateSignatureUnspecified otherwise (the safe zero).
	Signature GateSignature
	// FaultClass mirrors the corresponding outcome: FaultClassLowUtilization when
	// Fired, FaultClassAbstain otherwise. Informational only.
	FaultClass FaultClass
	// MFU and TensorActive are the supporting measured fractions.
	MFU          float64
	TensorActive float64
}

// LowUtilization evaluates the deterministic LOW_UTILIZATION rule for one
// device-efficiency result under a policy: it fires only when BOTH the MFU and
// the tensor-active fraction sit below their floors (the two independent
// underutilization conditions). This is the single source of truth for the
// low-utilization signature shape; rca/closed makes the binding gate call.
func LowUtilization(eff DeviceEfficiency, policy CostPolicy) LowUtilizationSignal {
	policy = policy.withDefaults()
	fired := eff.MFU < policy.MFUFloor && eff.TensorActive < policy.TensorActiveFloor
	sig := LowUtilizationSignal{
		Device:       eff,
		Fired:        fired,
		Signature:    GateSignatureUnspecified,
		FaultClass:   FaultClassAbstain,
		MFU:          eff.MFU,
		TensorActive: eff.TensorActive,
	}
	if fired {
		sig.Signature = GateSignatureLowUtilization
		sig.FaultClass = FaultClassLowUtilization
	}
	return sig
}
