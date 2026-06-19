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

// RankedDevice is one device's place in a wasted-$ ranking for a single window
// (TASK-0055, G4 money-story wedge). It carries the device identity, its window
// waste/burn-rate, and whether the device was priced — so an UNPRICED device is
// ranked honestly (Priced=false, WastedUSD/UsdPerHour=0) rather than fabricating
// a $ value. This is the deterministic input to the cli "TOP WASTED-$" digest.
type RankedDevice struct {
	Device Device
	// WastedUSD is the per-WINDOW wasted spend (CostWedge.WastedUSD). Zero when
	// the device is unpriced (Priced=false) — never a fabricated number.
	WastedUSD float64
	// UsdPerHour is the per-HOUR burn rate at the current idle fraction. Zero when
	// unpriced.
	UsdPerHour float64
	// IdleFraction is the window idle fraction (1-MFU), carried for context. It is
	// always meaningful (derived from MFU) regardless of pricing.
	IdleFraction float64
	// MFU is the device's measured model-FLOPs-utilization for the window.
	MFU float64
	// Priced is false when the device had no $/hour rate; then the $ fields are
	// zero and carry no meaning (the cli digest degrade-marks them, never $0).
	Priced bool
}

// WastedRanking is the result of TopWastedUSD for one window (TASK-0055): the
// devices ranked by wasted-$ descending plus the window totals.
//
// SCOPE: this is a SINGLE-WINDOW digest. A multi-window "wasted-$ last week"
// rollup needs a historical store to accumulate per-window waste over time,
// which is OUT OF SCOPE here (the remaining TASK-0055 follow-up). The totals
// below are for exactly the one window of CostWedges passed in. The $ values
// are only as real as the configured/spec $/hour rate (an operator/infra input,
// placeholder until the operator supplies it) and stay device-level because
// vanilla DCGM carries no job label for job-level attribution.
type WastedRanking struct {
	// Ranked is every input device, ordered by WastedUSD descending with a stable
	// tie-break by device UUID ascending. Unpriced devices (WastedUSD=0) sort to
	// the end among the zero-waste devices, still UUID-ordered.
	Ranked []RankedDevice
	// TotalWastedUSD sums WastedUSD across all PRICED devices in the window.
	TotalWastedUSD float64
	// TotalUsdPerHour sums UsdPerHour across all PRICED devices: the window's
	// aggregate idle burn rate.
	TotalUsdPerHour float64
	// AnyPriced is true when at least one device had a $/hour rate. When false the
	// totals are zero and carry no meaning — the digest says so instead of "$0".
	AnyPriced bool
	// Priced / Unpriced count the devices in each pricing state, so the digest can
	// honestly state coverage (e.g. "3 of 5 devices unpriced").
	Priced   int
	Unpriced int
}

// TopWastedUSD ranks the per-device cost wedges of ONE window by wasted-$
// descending and sums the window totals (TASK-0055, G4 money-story wedge). It is
// a pure, deterministic function: identical wedges in -> byte-identical ranking
// out, with a stable tie-break by device UUID ascending (so equal-waste devices
// — notably the many $0 / unpriced ones — order reproducibly).
//
// It honors the existing unpriced semantics from costWedgeFromEff: a wedge whose
// CostImpact was not Computed (no $/hour rate) contributes 0 to the totals and is
// marked Priced=false, NEVER a fabricated $. Idle/MFU context is still carried
// for every device since those are derived from measured FLOPs, not from price.
//
// SCOPE (see WastedRanking): single-window only; multi-window history, the real
// $/hour rate, and job-level attribution are out of scope / operator inputs.
func TopWastedUSD(wedges []CostWedge) WastedRanking {
	out := WastedRanking{Ranked: make([]RankedDevice, 0, len(wedges))}
	for _, w := range wedges {
		priced := w.Impact.Computed
		rd := RankedDevice{
			Device:       w.Device.Device,
			IdleFraction: w.IdleFraction,
			MFU:          w.Device.MFU,
			Priced:       priced,
		}
		if priced {
			rd.WastedUSD = w.WastedUSD
			rd.UsdPerHour = w.Impact.UsdPerHour
			out.TotalWastedUSD += w.WastedUSD
			out.TotalUsdPerHour += w.Impact.UsdPerHour
			out.Priced++
			out.AnyPriced = true
		} else {
			out.Unpriced++
		}
		out.Ranked = append(out.Ranked, rd)
	}
	// Deterministic: wasted-$ descending, stable tie-break by UUID ascending.
	sort.Slice(out.Ranked, func(i, j int) bool {
		if out.Ranked[i].WastedUSD != out.Ranked[j].WastedUSD {
			return out.Ranked[i].WastedUSD > out.Ranked[j].WastedUSD
		}
		return out.Ranked[i].Device.UUID < out.Ranked[j].Device.UUID
	})
	return out
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
