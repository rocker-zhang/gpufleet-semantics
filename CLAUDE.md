# CLAUDE.md — gpufleet-semantics (module session rules)

You are a Claude session **scoped to this repo only** (`gpufleet-semantics`).
This is an OPEN module (Apache-2.0). Your edits are **confined to this repo**.

## What this module is

A pure Go library: device→job mapping types + deterministic cost/efficiency
math (MFU, tensor-active, straggler, `$` cost). No collection, no GPU, no LLM,
no network. The foundation other modules depend on.

## Hard boundaries (do not cross)

- **Edits confined here.** Never edit another module's repo. If a task needs a
  change in `agent`, `rca`, `cli`, or the control plane, **ABSTAIN** and report a
  blocker — do not reach across.
- **`proto/` is READ-ONLY.** You may read the vendored proto contracts. You may
  **never** edit them. If a task requires a contract change (new field, new
  message, changed semantics), STOP and file a *contract change proposal*
  blocker for the orchestrator — module sessions never edit proto.
- **Determinism-first.** Every function here must be pure and reproducible. No
  `time.Now()`, no maps iterated for output without sorting, no randomness.
- **Read-only / off-critical-path.** Nothing here may run in a job-exec path or
  control a GPU.
- **No externally-sourced content.** No copied code, data, or proprietary
  error-code semantics from any prior work.

## If you are blocked

Write a short blocker note (what you needed, which other module/contract it
touches) and stop. Do not implement a workaround that edits outside this repo.

## Definition of done

`go test -race ./...` and `go vet ./...` pass; new logic has a deterministic
table/fixed-input unit test.
