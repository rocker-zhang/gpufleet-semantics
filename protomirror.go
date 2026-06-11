package semantics

// ----------------------------------------------------------------------------
// proto contract mirror (thin, hand-rolled)
//
// This file mirrors the SHAPES of the open `gpufleet.v1` contracts that this
// library produces/consumes, so the pure math here can speak the boundary
// vocabulary WITHOUT taking a hard build dependency on the generated proto
// module today.
//
// Why a mirror and not a direct import (RULES §D, CONTRACTS.md §4):
//   - CONTRACTS.md §4 says Go consumers vendor the contracts at a *published,
//     fixed tag* (`require github.com/.../gen/go vX.Y.Z`). At the time of this
//     task the proto module is generated but not yet published/tagged, and its
//     baseline is not yet clean on main (buf breaking). Pinning a floating ref
//     or a local `replace` to an absolute path is explicitly disallowed.
//   - This module must stay buildable and testable as a pure, standalone lib
//     and must NOT block on proto tooling (per the task card).
//
// The numbers/field names below are copied 1:1 from
//   proto/schema/gpufleet/v1/{verdict,signal}.proto
// and verified against proto/gen/go. They are kept additive-only and in the
// same wire order so the switch-over is mechanical.
//
// TODO(proto): once `gpufleet/proto/gen/go` is published at a fixed tag, drop
// this file, add `require github.com/rocker-zhang/gpufleet-proto/gen/go vX.Y.Z`
// to go.mod, and alias these types to the generated ones
// (e.g. `type CostImpact = gpufleetv1.CostImpact`) so there is exactly one
// definition of the boundary shape. See CONTRACTS.md §4 / D-0008.
// ----------------------------------------------------------------------------

// SignalSource is the independence class used by the >=2-corroborating-signal
// gate: two facts from the same source do NOT corroborate. Mirror of
// gpufleet.v1.SignalSource (signal.proto). Numbers are load-bearing.
type SignalSource int32

const (
	SignalSourceUnspecified SignalSource = 0
	SignalSourceDCGM        SignalSource = 1
	SignalSourceDmesgXID    SignalSource = 2
	SignalSourceNCCL        SignalSource = 3
	SignalSourcePrometheus  SignalSource = 4
	SignalSourceScheduler   SignalSource = 5
	SignalSourceProc        SignalSource = 6
)

// FaultClass is the closed deterministic outcome set. Only the subset this
// library can deterministically reason about (the standalone cost wedge) is
// mirrored; the rest of the enum is owned by the gate. Mirror of
// gpufleet.v1.FaultClass (verdict.proto). Numbers are load-bearing.
type FaultClass int32

const (
	FaultClassUnspecified    FaultClass = 0
	FaultClassAbstain        FaultClass = 1
	FaultClassLowUtilization FaultClass = 9
)

// GateSignature is the versioned signature-id registry: audit metadata only,
// NEVER an input to the class decision. Mirror of gpufleet.v1.GateSignature
// (verdict.proto). Numbers are load-bearing.
type GateSignature int32

const (
	GateSignatureUnspecified    GateSignature = 0
	GateSignatureLowUtilization GateSignature = 6
)

// CostImpact is the deterministic $ attribution for a window. Absent/zero means
// "could not be computed", never "free". Mirror of gpufleet.v1.CostImpact
// (verdict.proto): field names and order match the proto exactly.
type CostImpact struct {
	// Computed reports whether a cost could be computed at all (mappings were
	// priced). When false, the USD fields are zero and carry no meaning.
	Computed bool
	// UsdWindow is the wasted/at-risk spend attributed over the window, in USD.
	UsdWindow float64
	// UsdPerHour is the extrapolated USD/hour burn rate if the condition
	// persists.
	UsdPerHour float64
	// Basis is an optional human-readable basis string, e.g.
	// "4 GPUs idle x $2.50/hr x 0.5h". Deterministic given the same inputs.
	Basis string
}
