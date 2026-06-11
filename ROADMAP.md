# semantics — module roadmap (M1..M6)

Module-local breakdown of what **this** library delivers each milestone. Mirrors
`../ops/PLAN.md`; `semantics` is a shared, pure, deterministic Go library
(device→job + cost/MFU/efficiency math). It does no collection, touches no GPU,
makes no network calls. Most milestones only touch this module lightly — M1 is
the heavy one.

---

## M1 — contracts + semantic foundation  (PRIMARY)
Deliver the device→job mapping types and the core attribution math: per-device
MFU = achieved FLOP/s ÷ peak FLOP/s (clamped `[0,1]`), tensor-active fraction,
`$` cost per window; per-job aggregation with mean MFU, straggler ratio
`(maxMFU−minMFU)/maxMFU`, and total cost. Consumes proto v0.1.0 types.

**Exit:** `go test -race ./...` + `go vet ./...` green; every exported function
has a deterministic table/fixed-input test; output is byte-for-byte reproducible
(sorted by device UUID); no I/O / GPU / network / `time.Now()` / randomness.

## M2 — wedge-ready attribution API
Stabilize the surface the `agent` calls to build the cost/efficiency portion of
the evidence pack for the "money story" demo (per-job MFU + wasted `$`). Lock
input/output struct shapes and error contract (`ErrBadWindow`, `ErrBadPeak`).

**Exit:** API consumed unchanged by `agent` for demo1; no breaking change needed
mid-milestone; the cost wedge math stands alone without any RCA dependency
(TASK-0015 contract honored from this side).

## M3 — independent signal for the gate
Expose straggler ratio (and supporting per-device MFU spread) as a clean,
documented independent signal `rca` can use toward its ≥2-signal gate.
Reconcile the straggler-ratio definition with the benchmark corpus so the math here is
the single source of truth and never contradicts the gate (TASK-0013).

**Exit:** straggler definition documented + tested; benchmark corpus values match this
library's output for the same inputs; no formula divergence between corpus and
gate.

## M4 — benchmark-stable numerics
Pin the MFU and `$`-cost numeric definitions (rounding, clamping, dtype-peak
selection contract) so the public benchmark scorecard and the closed eval are
reproducible. Add fixed-vector regression tests that fail on silent formula
drift.

**Exit:** numeric definitions documented as stable; regression vectors in place;
benchmark gate can rely on these outputs without flakiness.

## M5 — reused server-side, no fork
Confirm the closed `controlplane` reuses this exact library for fleet-level cost
attribution (no reimplementation, no fork). Keep the public API additive only so
the moat builds on top rather than around it.

**Exit:** controlplane links this library for fleet cost rollups; any needed
extension is additive and lands here in the open repo; zero forked math.

## M6 — partner validation
Validate the cost-attribution math against real GPU fleets (GB10 /
4×A10) via partners — peak-FLOPS specs and `$`/hour inputs exercised on
real device mixes; correct any spec/clamping edge cases found.

**Exit:** cost-attribution numbers reviewed against real fleet billing on
partner hardware; edge cases (multi-model jobs, idle windows) covered by tests.
