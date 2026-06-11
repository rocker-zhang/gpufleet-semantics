package semantics

import "testing"

// TestStandaloneCostNoFaultSignals proves the cost wedge is computed with ZERO
// fault/RCA signals (TASK-0015): a plain measurement window, no gate, no fault,
// still yields a non-zero CostImpact attributing idle dollars.
func TestStandaloneCostNoFaultSignals(t *testing.T) {
	// 0.5h window, MFU 0.1 -> 90% idle. Cost 2.50/hr -> CostUSD = 1.25.
	// Wasted = 1.25 * 0.9 = 1.125. UsdPerHour = 2.50 * 0.9 = 2.25.
	s := DeviceSample{
		Device:           Device{UUID: "GPU-idle", Node: "n1", Model: "A10"},
		WindowSeconds:    1800,
		AchievedFLOPs:    0.1 * 5e13 * 1800, // achieved rate = 0.1*peak
		TensorActiveSecs: 0.05 * 1800,
	}
	spec := DeviceSpec{PeakFLOPS: 5e13, CostPerHour: 2.50}

	w, err := DeviceCostWedge(s, spec, DefaultCostPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !w.Impact.Computed {
		t.Fatalf("cost must be computed for a priced window; got Computed=false")
	}
	if !almost(w.Device.MFU, 0.1) {
		t.Errorf("MFU = %v, want 0.1", w.Device.MFU)
	}
	if !almost(w.IdleFraction, 0.9) {
		t.Errorf("IdleFraction = %v, want 0.9", w.IdleFraction)
	}
	if !almost(w.WastedUSD, 1.125) {
		t.Errorf("WastedUSD = %v, want 1.125", w.WastedUSD)
	}
	if !almost(w.Impact.UsdWindow, 1.125) {
		t.Errorf("Impact.UsdWindow = %v, want 1.125", w.Impact.UsdWindow)
	}
	if !almost(w.Impact.UsdPerHour, 2.25) {
		t.Errorf("Impact.UsdPerHour = %v, want 2.25", w.Impact.UsdPerHour)
	}
	if w.Impact.Basis == "" {
		t.Errorf("Basis should be populated for a priced window")
	}
	// Low MFU AND low tensor-active -> low-utilization condition holds.
	if !w.LowUtilization {
		t.Errorf("expected LowUtilization=true for MFU=0.1, tensor=0.05")
	}
}

// TestCostWedgeHealthyZeroWasted: a fully utilized device wastes nothing, yet
// cost is still "computed" (the path always runs).
func TestCostWedgeHealthyZeroWasted(t *testing.T) {
	s := DeviceSample{
		Device:           Device{UUID: "GPU-busy"},
		WindowSeconds:    3600,
		AchievedFLOPs:    5e13 * 3600, // MFU = 1.0
		TensorActiveSecs: 3600,
	}
	w, err := DeviceCostWedge(s, DeviceSpec{PeakFLOPS: 5e13, CostPerHour: 4.0}, DefaultCostPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !w.Impact.Computed {
		t.Errorf("Computed must be true for a priced window")
	}
	if w.IdleFraction != 0 {
		t.Errorf("IdleFraction = %v, want 0", w.IdleFraction)
	}
	if w.WastedUSD != 0 {
		t.Errorf("WastedUSD = %v, want 0", w.WastedUSD)
	}
	if w.LowUtilization {
		t.Errorf("healthy device must not flag LowUtilization")
	}
}

// TestCostWedgeUnpricedNotComputed: no cost rate -> Computed=false, zero USD,
// but the wedge (idle fraction) is still produced.
func TestCostWedgeUnpricedNotComputed(t *testing.T) {
	s := DeviceSample{
		Device:           Device{UUID: "GPU-x"},
		WindowSeconds:    100,
		AchievedFLOPs:    0, // fully idle
		TensorActiveSecs: 0,
	}
	w, err := DeviceCostWedge(s, DeviceSpec{PeakFLOPS: 1e12, CostPerHour: 0}, DefaultCostPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Impact.Computed {
		t.Errorf("Computed must be false when no cost rate is known")
	}
	if w.WastedUSD != 0 || w.Impact.UsdWindow != 0 || w.Impact.UsdPerHour != 0 {
		t.Errorf("unpriced window must carry zero USD, got %+v", w.Impact)
	}
	if !almost(w.IdleFraction, 1.0) {
		t.Errorf("IdleFraction = %v, want 1.0 for fully idle", w.IdleFraction)
	}
	if w.Impact.Basis != "" {
		t.Errorf("unpriced window must have empty Basis, got %q", w.Impact.Basis)
	}
}

// TestCostWedgeErrors: clamp/error contract is inherited from DeviceEff.
func TestCostWedgeErrors(t *testing.T) {
	if _, err := DeviceCostWedge(DeviceSample{WindowSeconds: 0}, DeviceSpec{PeakFLOPS: 1}, CostPolicy{}); err != ErrBadWindow {
		t.Errorf("want ErrBadWindow, got %v", err)
	}
	if _, err := DeviceCostWedge(DeviceSample{WindowSeconds: 1}, DeviceSpec{PeakFLOPS: 0}, CostPolicy{}); err != ErrBadPeak {
		t.Errorf("want ErrBadPeak, got %v", err)
	}
}

// TestJobCostWedgeDeterministic: aggregate across a job, sorted by UUID, sums
// wasted spend; the mixed priced/unpriced case still reports Computed=true.
func TestJobCostWedgeDeterministic(t *testing.T) {
	job := Job{ID: "job-9", Owner: "team-b"}
	devs := []DeviceEfficiency{
		// out of order on purpose
		{Device: Device{UUID: "GPU-z"}, MFU: 0.5, TensorActive: 0.5, CostUSD: 2.0},
		{Device: Device{UUID: "GPU-a"}, MFU: 0.0, TensorActive: 0.0, CostUSD: 2.0},
	}
	specs := map[string]DeviceSpec{
		"GPU-z": {PeakFLOPS: 1e12, CostPerHour: 4.0},
		"GPU-a": {PeakFLOPS: 1e12, CostPerHour: 4.0},
	}
	jc := JobCostWedge(job, devs, specs, DefaultCostPolicy())

	if jc.Wedges[0].Device.Device.UUID != "GPU-a" {
		t.Errorf("wedges not sorted by UUID: first = %s", jc.Wedges[0].Device.Device.UUID)
	}
	// GPU-a: idle 1.0 -> wasted 2.0 ; GPU-z: idle 0.5 -> wasted 1.0 ; total 3.0
	if !almost(jc.Impact.UsdWindow, 3.0) {
		t.Errorf("job UsdWindow = %v, want 3.0", jc.Impact.UsdWindow)
	}
	if !jc.Impact.Computed {
		t.Errorf("job cost must be computed")
	}
}

// TestLowUtilizationSignatureMirror: the deterministic LOW_UTILIZATION rule
// fires only when BOTH conditions hold and carries the mirrored signature id.
func TestLowUtilizationSignatureMirror(t *testing.T) {
	pol := DefaultCostPolicy()

	// Both low -> fires.
	fired := LowUtilization(DeviceEfficiency{Device: Device{UUID: "g1"}, MFU: 0.1, TensorActive: 0.05}, pol)
	if !fired.Fired {
		t.Fatalf("expected fire when both MFU and tensor below floor")
	}
	if fired.Signature != GateSignatureLowUtilization {
		t.Errorf("Signature = %v, want GateSignatureLowUtilization(6)", fired.Signature)
	}
	if fired.FaultClass != FaultClassLowUtilization {
		t.Errorf("FaultClass = %v, want FaultClassLowUtilization(9)", fired.FaultClass)
	}

	// Low MFU but HIGH tensor-active (single condition) -> does NOT fire, stays
	// at the safe-zero signature/abstain (>=2 independent conditions required).
	one := LowUtilization(DeviceEfficiency{MFU: 0.1, TensorActive: 0.9}, pol)
	if one.Fired {
		t.Errorf("must not fire on a single condition")
	}
	if one.Signature != GateSignatureUnspecified {
		t.Errorf("Signature = %v, want GateSignatureUnspecified(0)", one.Signature)
	}
	if one.FaultClass != FaultClassAbstain {
		t.Errorf("FaultClass = %v, want FaultClassAbstain(1)", one.FaultClass)
	}
}

// TestProtoMirrorEnumNumbers pins the mirrored enum numbers to the proto
// contract values; a drift here means the mirror diverged from the contract.
func TestProtoMirrorEnumNumbers(t *testing.T) {
	cases := []struct {
		name string
		got  int32
		want int32
	}{
		{"SignalSourceUnspecified", int32(SignalSourceUnspecified), 0},
		{"SignalSourceDCGM", int32(SignalSourceDCGM), 1},
		{"SignalSourceNCCL", int32(SignalSourceNCCL), 3},
		{"SignalSourceProc", int32(SignalSourceProc), 6},
		{"FaultClassUnspecified", int32(FaultClassUnspecified), 0},
		{"FaultClassAbstain", int32(FaultClassAbstain), 1},
		{"FaultClassLowUtilization", int32(FaultClassLowUtilization), 9},
		{"GateSignatureUnspecified", int32(GateSignatureUnspecified), 0},
		{"GateSignatureLowUtilization", int32(GateSignatureLowUtilization), 6},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d (proto-contract drift)", c.name, c.got, c.want)
		}
	}
}
