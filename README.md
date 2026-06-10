# gpufleet-semantics

Apache-2.0 · OPEN module · `github.com/rocker-zhang/gpufleet-semantics`

The **adoption wedge** of gpufleet and the foundation every other module builds
on: deterministic device→job mapping plus cost/efficiency math (MFU,
tensor-active fraction, straggler ratio, $ cost attribution).

This is a pure Go library. It does **NOT** collect metrics and does **NOT**
touch a GPU — callers (the `agent`) hand it already-normalized numbers and it
returns deterministic attribution. No LLM, no network, no I/O.

## What it computes

- `DeviceEff` — per-device MFU = achieved FLOP/s ÷ peak FLOP/s (clamped to
  `[0,1]`), tensor-active fraction, and `$` cost for a measurement window.
- `JobEff` — aggregates devices up to a job: mean MFU, **straggler ratio**
  `(maxMFU − minMFU) / maxMFU`, and total cost. Output is sorted by device UUID
  so results are byte-for-byte reproducible.

## Usage

```go
eff, err := semantics.DeviceEff(sample, spec)
job := semantics.JobEff(theJob, []semantics.DeviceEfficiency{eff, ...})
```

## Boundaries

- Read-only, off-critical-path: this code never runs in a job-execution path and
  never controls/orchestrates/checkpoints GPUs.
- Determinism-first: same inputs → same outputs, always.
- Contracts (`proto/`) are a read-only dependency. This repo never edits proto.

## Develop

```sh
go test -race ./...
go vet ./...
```

CI runs lint, a test matrix including `ubuntu-24.04-arm` with `-race`, cross
builds for amd64+arm64 with `CGO_ENABLED=0`, govulncheck, a syft SBOM, and
gitleaks. arm64 is a first-class CI target.
