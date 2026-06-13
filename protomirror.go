package semantics

import (
	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
)

// ----------------------------------------------------------------------------
// proto contract boundary (TASK-0029).
//
// The open `gpufleet.v1` contracts are now published at a fixed tag and vendored
// read-only (require .../gen/go v0.1.0 + local replace => ../proto/gen/go, the
// same convention agent uses). Per TASK-0017 DoD#3 / TASK-0029 this module now
// consumes the REAL generated types for the boundary ENUMS, so there is exactly
// ONE definition of each enum shape (the generated one) — the hand-rolled enum
// mirror is gone.
//
//   - SignalSource / FaultClass / GateSignature are plain int32 enums in gen.
//     They are exposed here as `type` ALIASES to the generated types, with the
//     local constant names kept as aliases to the generated enum values. This
//     keeps semantics' exported API byte-for-byte stable (same type identities,
//     same constant names) while removing the duplicate definition: a value
//     produced here is literally a gpufleetv1.SignalSource, etc.
//
// CostImpact is deliberately NOT aliased to gpufleetv1.CostImpact — see the note
// on the CostImpact type below for the precise, proven reason.
// ----------------------------------------------------------------------------

// SignalSource is the independence class used by the >=2-corroborating-signal
// gate: two facts from the same source do NOT corroborate. This is an ALIAS to
// the generated gpufleet.v1.SignalSource — one definition, in the contract.
type SignalSource = gpufleetv1.SignalSource

const (
	SignalSourceUnspecified = gpufleetv1.SignalSource_SIGNAL_SOURCE_UNSPECIFIED
	SignalSourceDCGM        = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
	SignalSourceDmesgXID    = gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID
	SignalSourceNCCL        = gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL
	SignalSourcePrometheus  = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS
	SignalSourceScheduler   = gpufleetv1.SignalSource_SIGNAL_SOURCE_SCHEDULER
	SignalSourceProc        = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROC
)

// FaultClass is the closed deterministic outcome set. This library only ever
// produces the subset it can deterministically reason about (the standalone
// cost wedge: UNSPECIFIED / ABSTAIN / LOW_UTILIZATION); the rest of the enum is
// owned by the gate. ALIAS to the generated gpufleet.v1.FaultClass.
type FaultClass = gpufleetv1.FaultClass

const (
	FaultClassUnspecified    = gpufleetv1.FaultClass_FAULT_CLASS_UNSPECIFIED
	FaultClassAbstain        = gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN
	FaultClassLowUtilization = gpufleetv1.FaultClass_FAULT_CLASS_LOW_UTILIZATION
)

// GateSignature is the versioned signature-id registry: audit metadata only,
// NEVER an input to the class decision. ALIAS to gpufleet.v1.GateSignature.
type GateSignature = gpufleetv1.GateSignature

const (
	GateSignatureUnspecified    = gpufleetv1.GateSignature_GATE_SIGNATURE_UNSPECIFIED
	GateSignatureLowUtilization = gpufleetv1.GateSignature_GATE_SIGNATURE_LOW_UTILIZATION
)

// CostImpact is the deterministic $ attribution for a window. Absent/zero means
// "could not be computed", never "free". Field names and order match
// gpufleet.v1.CostImpact (verdict.proto) exactly, so producing the proto message
// at the agent's serialization boundary is a mechanical field copy.
//
// Why this is a plain value struct and NOT `type CostImpact = gpufleetv1.CostImpact`
// (unlike the enums above):
//
//	gpufleetv1.CostImpact is a generated protobuf MESSAGE. It embeds
//	protoimpl.MessageState, which carries pragma.DoNotCopy ([0]sync.Mutex) and
//	pragma.DoNotCompare ([0]func()). semantics is a PURE VALUE library: CostImpact
//	is embedded by value in CostWedge / JobCostImpact and those are copied by
//	value pervasively (slice append, range, value returns) here AND in the agent
//	consumer. Aliasing CostImpact to the proto message makes `go vet` copylocks
//	fire on every such copy in BOTH modules (proven: "return copies lock value:
//	CostWedge contains CostImpact contains protoimpl.MessageState contains
//	sync.Mutex"), and the no-compare pragma forbids `==`. Unifying CostImpact onto
//	the gen message would therefore require changing CostWedge/JobCostImpact to
//	hold *CostImpact pointers — a public-API break that also touches the agent
//	module (out of scope for this card, RULES §D / semantics CLAUDE.md §6).
//
// The boundary shape is still single-sourced where it can be cleanly aliased
// (the three enums above). The remaining duplication is this one value struct,
// tracked as a blocker on TASK-0029 (see the module return note) for a follow-up
// that owns the cross-module pointer/refactor or a plain-struct proto variant.
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
