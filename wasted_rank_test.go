package semantics

import (
	"testing"
)

// wedge is a tiny helper that builds a CostWedge directly from fixed numbers so
// the ranking tests are fully deterministic (no FLOPs math, no wall-clock).
// priced=false models an unpriced device: Impact.Computed=false, $ fields zero.
func wedge(uuid string, wasted, perHour, idle, mfu float64, priced bool) CostWedge {
	w := CostWedge{
		Device:       DeviceEfficiency{Device: Device{UUID: uuid, Node: "n1"}, MFU: mfu},
		IdleFraction: idle,
	}
	if priced {
		w.WastedUSD = wasted
		w.Impact = CostImpact{Computed: true, UsdWindow: wasted, UsdPerHour: perHour}
	} else {
		// Unpriced: no $/hour rate. Impact not computed; $ fields zero.
		w.Impact = CostImpact{Computed: false}
	}
	return w
}

// TestTopWastedUSDRankAndTotals: priced devices rank by wasted-$ descending and
// the window totals sum only the priced devices. Deterministic input -> fixed
// order and fixed totals.
func TestTopWastedUSDRankAndTotals(t *testing.T) {
	in := []CostWedge{
		wedge("GPU-b", 0.50, 1.00, 0.50, 0.50, true),
		wedge("GPU-a", 2.00, 4.00, 0.90, 0.10, true),
		wedge("GPU-c", 1.00, 2.00, 0.70, 0.30, true),
	}
	r := TopWastedUSD(in)

	wantOrder := []string{"GPU-a", "GPU-c", "GPU-b"}
	if len(r.Ranked) != len(wantOrder) {
		t.Fatalf("Ranked len = %d, want %d", len(r.Ranked), len(wantOrder))
	}
	for i, u := range wantOrder {
		if r.Ranked[i].Device.UUID != u {
			t.Errorf("Ranked[%d].UUID = %q, want %q", i, r.Ranked[i].Device.UUID, u)
		}
	}
	if !almost(r.TotalWastedUSD, 3.50) {
		t.Errorf("TotalWastedUSD = %v, want 3.50", r.TotalWastedUSD)
	}
	if !almost(r.TotalUsdPerHour, 7.00) {
		t.Errorf("TotalUsdPerHour = %v, want 7.00", r.TotalUsdPerHour)
	}
	if !r.AnyPriced || r.Priced != 3 || r.Unpriced != 0 {
		t.Errorf("pricing counts: AnyPriced=%v Priced=%d Unpriced=%d, want true/3/0",
			r.AnyPriced, r.Priced, r.Unpriced)
	}
}

// TestTopWastedUSDStableTieBreak: devices with EQUAL wasted-$ (here all the $0
// unpriced ones) order by UUID ascending, reproducibly, regardless of input
// order.
func TestTopWastedUSDStableTieBreak(t *testing.T) {
	in := []CostWedge{
		wedge("GPU-z", 0, 0, 0.95, 0.05, true), // priced, $0 waste
		wedge("GPU-m", 0, 0, 0.10, 0.90, true), // priced, $0 waste
		wedge("GPU-a", 0, 0, 0.50, 0.50, true), // priced, $0 waste
	}
	r := TopWastedUSD(in)
	want := []string{"GPU-a", "GPU-m", "GPU-z"}
	for i, u := range want {
		if r.Ranked[i].Device.UUID != u {
			t.Errorf("Ranked[%d].UUID = %q, want %q", i, r.Ranked[i].Device.UUID, u)
		}
	}
}

// TestTopWastedUSDUnpricedHonest: an unpriced device contributes 0 to the totals,
// is marked Priced=false, and never gets a fabricated $ — it sorts among the
// $0 devices by UUID. A mix of priced + unpriced reports honest coverage counts.
func TestTopWastedUSDUnpricedHonest(t *testing.T) {
	in := []CostWedge{
		wedge("GPU-priced", 1.50, 3.00, 0.80, 0.20, true),
		wedge("GPU-noprice", 9.99, 9.99, 0.99, 0.01, false), // numbers ignored: unpriced
	}
	r := TopWastedUSD(in)

	if r.Ranked[0].Device.UUID != "GPU-priced" {
		t.Errorf("priced device should rank first, got %q", r.Ranked[0].Device.UUID)
	}
	np := r.Ranked[1]
	if np.Device.UUID != "GPU-noprice" {
		t.Fatalf("Ranked[1] = %q, want GPU-noprice", np.Device.UUID)
	}
	if np.Priced {
		t.Errorf("unpriced device must have Priced=false")
	}
	if np.WastedUSD != 0 || np.UsdPerHour != 0 {
		t.Errorf("unpriced device must carry $0 (no fabrication); got wasted=%v perHour=%v",
			np.WastedUSD, np.UsdPerHour)
	}
	// Idle/MFU context is still honest for the unpriced device.
	if !almost(np.IdleFraction, 0.99) || !almost(np.MFU, 0.01) {
		t.Errorf("unpriced device must keep MFU/idle context; got idle=%v mfu=%v",
			np.IdleFraction, np.MFU)
	}
	if !almost(r.TotalWastedUSD, 1.50) {
		t.Errorf("TotalWastedUSD = %v, want 1.50 (unpriced excluded)", r.TotalWastedUSD)
	}
	if r.Priced != 1 || r.Unpriced != 1 || !r.AnyPriced {
		t.Errorf("coverage: Priced=%d Unpriced=%d AnyPriced=%v, want 1/1/true",
			r.Priced, r.Unpriced, r.AnyPriced)
	}
}

// TestTopWastedUSDAllUnpriced: when NO device is priced, totals are zero and
// AnyPriced is false — the caller (cli digest) renders "no $/hour rate" instead
// of "$0". Devices still rank by UUID.
func TestTopWastedUSDAllUnpriced(t *testing.T) {
	in := []CostWedge{
		wedge("GPU-2", 0, 0, 0.5, 0.5, false),
		wedge("GPU-1", 0, 0, 0.9, 0.1, false),
	}
	r := TopWastedUSD(in)
	if r.AnyPriced {
		t.Errorf("AnyPriced should be false when nothing is priced")
	}
	if r.TotalWastedUSD != 0 || r.TotalUsdPerHour != 0 {
		t.Errorf("totals must be zero when nothing is priced")
	}
	if r.Unpriced != 2 || r.Priced != 0 {
		t.Errorf("coverage: Priced=%d Unpriced=%d, want 0/2", r.Priced, r.Unpriced)
	}
	if r.Ranked[0].Device.UUID != "GPU-1" || r.Ranked[1].Device.UUID != "GPU-2" {
		t.Errorf("unpriced devices should order by UUID; got %q,%q",
			r.Ranked[0].Device.UUID, r.Ranked[1].Device.UUID)
	}
}

// TestTopWastedUSDEmpty: no devices -> empty, zeroed, AnyPriced=false. No panic.
func TestTopWastedUSDEmpty(t *testing.T) {
	r := TopWastedUSD(nil)
	if len(r.Ranked) != 0 || r.AnyPriced || r.TotalWastedUSD != 0 {
		t.Errorf("empty input should yield empty zeroed ranking; got %+v", r)
	}
}
