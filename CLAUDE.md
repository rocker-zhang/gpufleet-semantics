# semantics — module brief (CLAUDE.md)

## 1. 身份
- **Class:** OPEN (Apache-2.0).
- **Language:** Go.
- **Kind:** library (shared library — NOT a tier, per D-0008).
- **Purpose (one line):** deterministic device→job mapping + cost/MFU/efficiency math, computed from already-normalized numbers.
- **Posture:** shared-lib. Pure, off-path, no I/O, no GPU, no network, no LLM. Reused on both the open side (agent, optionally cli) and the closed side (controlplane).

## 2. 在系统里的位置
- **Consumes:** plain numeric inputs handed in by callers (windowed device samples, peak-FLOPS / cost specs). It does **no** collection itself. Where types cross a wire they conform to the open `proto/` contracts (signal/verdict/plugin); this repo treats `proto/` as a read-only dependency.
- **Produces:** `DeviceEfficiency` (MFU, tensor-active fraction, `$` cost) and `JobEfficiency` (mean MFU, straggler ratio, total cost), sorted deterministically (by device UUID) so output is byte-for-byte reproducible.
- **Edges:**
  - `agent` (the data plane) collects + normalizes per D-0009, then calls this library to attribute cost/efficiency for the evidence pack.
  - `rca` consumes the deterministic numbers (e.g. straggler ratio) as one independent signal toward its ≥2-signal gate.
  - `controlplane` (closed) reuses this same library for fleet-level cost attribution — no fork, no reimplementation.
  - `cli` (open viewer) may optionally use it to render single-node attribution from the agent's local API.
- See `../ARCHITECTURE.md` for the full system map and `../ops/DECISIONS.md` for D-0008..D-0011.

## 3. 继承的红线
Inherits all of `../RULES.md`. Module-specific hard lines:
- **Pure library — no I/O, no side effects, no network, no GPU, no LLM.** Every exported function is deterministic: same inputs → same outputs, always. No `time.Now()`, no unsorted map iteration in output, no randomness.
- **Read-only & off-critical-path.** Nothing here may run in a job-exec path, control/checkpoint/orchestrate a GPU, or block a node. Inputs are numbers the agent already collected.
- **`proto/` is READ-ONLY to this module.** Never edit a contract. proto is semver + buf-breaking governed; a contract change (new field/message/changed semantics) goes via a proposal to the orchestrator, never from here.
- **No externally-sourced content.** No copied code/data/proprietary error-code semantics from any prior work; provenance is personal hardware + personal time only.

## 4. 当前任务 & 里程碑焦点
- Focus milestone: **M1 — contracts + semantic foundation.** This module is the "semantic foundation" half: device→job + MFU / tensor-active / straggler / `$`cost math with deterministic unit tests, consuming proto v0.1.0.
- Relevant `../ops/BOARD.md` cards:
  - **TASK-0006** — semantics: device→job mapping + MFU / tensor-active / straggler + `$`cost math; deterministic passing test (this module).
  - **TASK-0015** — cost wedge as standalone deterministic path: the math here must stand alone as the cost/efficiency wedge independent of RCA (consumer-side, but the math contract lives here).
  - **TASK-0013** — straggler corpus must not contradict the gate: keep the straggler-ratio definition here the single source of truth that the corpus and gate agree on.

## 5. 构建与测试
- Always `source ../.envrc` first (project-local GOPATH/GOBIN/caches under `./.tools` & `./.cache`; no global pollution — RULES §J).
- Build/test:
  ```sh
  source ../.envrc
  go build ./...
  go vet ./...
  go test -race ./...
  ```
- **DoD:** `go test -race ./...` and `go vet ./...` pass; new logic carries a deterministic table / fixed-input unit test.
- **CI (one line):** lint + test matrix (incl. `ubuntu-24.04-arm` with `-race`), cross-build amd64+arm64 `CGO_ENABLED=0`, govulncheck, syft SBOM, gitleaks; arm64 is first-class.

## 6. session 工作规则
- **Edits confined to this repo.** Never edit `agent`, `rca`, `cli`, `controlplane`, or any other module.
- **proto is read-only.** Read vendored contracts; never modify.
- **Abstain + report blocker** if a task needs a contract change or another module: write a short blocker note (what was needed, which module/contract it touches) and stop — do not implement a cross-repo workaround.
- **Provenance:** personal hardware, personal time; no externally-sourced content.

## 6a. Boundary-shape single-sourcing & the CostImpact value mirror (TASK-0029 / TASK-0035)

The open `gpufleet.v1` contracts are vendored read-only; this module consumes the
**real generated types** for the boundary shapes so each shape has one definition.

- **Enums** (`SignalSource` / `FaultClass` / `GateSignature`) are `type` **aliases**
  to the gen enums (plain `int32`), so a value here *is* the gen value. Their wire
  numbers are pinned by `TestProtoEnumNumbers`.
- **`CostImpact`** is **deliberately NOT aliased** — it is a documented plain
  **value-type mirror** of `gpufleet.v1.CostImpact`. This is the accepted
  **option A** of TASK-0035, not an accidental duplicate.
  - *Why no alias:* the gen `CostImpact` is a protobuf **message** embedding
    `protoimpl.MessageState` (`sync.Mutex` via `pragma.DoNotCopy`, plus
    `DoNotCompare`). semantics is a pure value-math lib that embeds `CostImpact`
    by value in `CostWedge`/`JobCostImpact` and copies it pervasively (slice
    append, range, value return) here **and** in the agent consumer. Aliasing
    makes `go vet` **copylocks** fire in both modules, and the no-compare pragma
    forbids `==`. Unifying onto the gen message would require `*CostImpact`
    pointers — a **public-API break that also touches `agent`** (options B/C),
    out of scope for this card.
  - *Anti-drift:* because the two definitions are physically distinct, they are
    pinned **field-by-field** by `TestCostImpactProtoFieldParity` — Go field
    name + type, protobuf wire **number** + **kind**, and **JSON name**, read
    from the gen message's own protoreflect descriptor (the canonical source of
    wire/JSON semantics). Any rename / renumber / kind change / add / remove on
    either side fails the test loudly, so the agent's mechanical field-copy at the
    serialization boundary stays correct.
  - The decision + proof also live as a code comment on the `CostImpact` type in
    `protomirror.go`.

## 7. 模块路线图
- **M1** — device→job + MFU/tensor-active/straggler/`$`cost math, deterministic unit tests, consumes proto v0.1.0. (PRIMARY milestone for this module.)
- **M2** — stabilize the attribution API consumed by `agent` for the evidence pack; lock the cost/MFU output shape behind the "money story" demo.
- **M3** — expose straggler ratio as a clean independent signal for `rca`'s ≥2-signal gate; reconcile with the benchmark corpus so math and gate never disagree.
- **M4** — pin numeric definitions (MFU/cost) so the public benchmark scorecard and closed eval are reproducible; no silent formula drift.
- **M5** — same library reused by the closed `controlplane` for fleet cost attribution; no fork.
- **M6** — validate cost-attribution math against real GPU fleets (GPU mix) via partners.

See `./ROADMAP.md` for module-local exit criteria.
