# TEMP Finding: Relia Architecture Budget And Decomposition

Date: 2026-06-30
Status: source finding; not dispatchable
Repo: Relia

## Boundary

This file is repo-local source evidence for future Factory/factoryd planning.
It is not a generated execution contract, task packet, acceptance ledger, or
scope-closure artifact. Before implementation starts, this finding must be
ingested or promoted through the governed Factory path so runner-ready task
packets, validation commands, lifecycle evidence, and acceptance refs are
materialized.

## Objective

Adopt architecture budget rules and decompose the current CLI monolith before
more Relia feature work expands the same file.

## Current Finding

Relia product behavior is concentrated in `cmd/relia/main.go`, while the repo
contract already names `cmd/relia/` as the CLI entrypoint and `internal/` as
the implementation package surface. That mismatch increases review churn,
repair blast radius, and merge risk.

## Workstream A: Repo-Pack Adoption

1. Update `AGENTS.md`, `docs/dev/dev_guides.md`, and
   `docs/architecture/architecture_guides.md` with explicit architecture
   budgets.
2. Add a validation surface for source file size inventory when Factory/factoryd
   exposes the shared budget contract.
3. Define the exception path for architecture debt:
   - affected file/package
   - reason
   - owner
   - expiry or follow-up task
   - compensating validation
4. Require future task packets touching oversized files to shrink, split, or
   carry an approved exception.

## Workstream B: CLI Decomposition

1. Move command parsing and process exit wiring into a thin `cmd/relia` layer.
2. Move product behavior into bounded `internal/*` packages.
3. Split tests by package responsibility.
4. Preserve command-result JSON, exit-code, schema, and artifact compatibility.
5. Keep Factory artifacts and closure evidence untouched except for
   task-scoped evidence required by the future decomposition task.

Current progress:

- `internal/diffparse`, `internal/yamlmini`, `internal/config`,
  `internal/result`, `internal/ingest`, and `internal/assess` now own bounded
  implementation slices extracted from the original CLI monolith.
- Unified-diff assessment parser coverage now lives in
  `cmd/relia/diffparse_test.go`, and advisory command, workflow, and state
  coverage now lives in `cmd/relia/advise_test.go`, leaving the tests in
  package `main` for unexported helper access.
- Ingest command safety and contract coverage now lives in
  `cmd/relia/ingest_security_test.go` and
  `cmd/relia/ingest_contract_test.go`, continuing to shrink
  `cmd/relia/main_test.go` without changing command behavior.
- Backtest report and integrity coverage now lives in
  `cmd/relia/backtest_report_test.go` and
  `cmd/relia/backtest_integrity_test.go`, keeping the extracted test files
  below the architecture warning budget.
- `internal/advise` now owns advisory comment decision, strategy, and rendering
  helpers with direct package tests, shrinking `cmd/relia/main.go` while
  keeping CLI I/O and state-file handling in the command layer.
- `internal/modelpull` now owns `relia models pull` argument parsing and local
  model manifest construction with direct package tests, while command I/O and
  repository writes remain in `cmd/relia`.
- `internal/distill` now owns provider-backed distill planning, adapter request
  shapes, cost estimation, and distill settings helpers with direct package
  tests, while CLI orchestration and provider-gate error rendering remain in
  `cmd/relia`.
- `internal/distill` now also owns `relia distill` argument parsing with
  direct package tests, while command-result formatting and repository I/O
  remain in `cmd/relia`.
- `internal/review` now owns `relia review` argument parsing and
  repo-relative scope-path validation with direct package tests, while memory
  rule lookup, lifecycle file updates, validation, and command-result rendering
  remain in `cmd/relia`.
- `internal/memory` now owns `relia memory` argument parsing and
  repo-relative output-path validation with direct package tests, while memory
  rule summary loading, Markdown rendering, artifact writes, and command-result
  assembly remain in `cmd/relia`.
- `internal/serve` now owns `relia serve` argument parsing and hosted transport
  dependency gating with direct package tests, while rule loading, served-rule
  data assembly, and MCP manifest rendering remain in `cmd/relia`.
- `internal/assess` now owns `relia assess` argument parsing with direct
  package tests, while diff reads, rule loading, risk assessment invocation,
  and command-result rendering remain in `cmd/relia`.
- `internal/ingest` now owns `relia ingest` argument parsing with direct
  package tests, while input file reads, event normalization, shard writes, and
  command-result rendering remain in `cmd/relia`.
- `internal/backtest` now owns `relia backtest` argument parsing, recurrence
  window duration validation, and the recurrence report data model with direct
  package tests, while experience loading, report metric assembly, baseline
  mutation, and command-result rendering remain in `cmd/relia`.

## Candidate Package Boundaries

- `internal/result`: command-result envelope and generic pass/error helpers
  extracted; command-specific error translation and rendering still live in
  `cmd/relia`.
- `internal/config`: default config rendering, config document references,
  config loading and validation, provider/advisory configuration parsing,
  local model-manifest dependency validation, and repo-relative path policy.
- `internal/ingest`: ingest input parsing, fail-closed redaction, standard
  secret-token scanning, provenance URL token-shape checks, the
  experience-record data model, canonical distill input decoding, record
  validation, shard persistence, and record/provenance URL helpers extracted;
  experience-record normalization and ingest result assembly still live in
  `cmd/relia`.
- `internal/backtest`: backtest command argument parsing, recurrence window
  validation, recurrence report model extracted; recurrence metrics and report
  generation remain candidates.
- `internal/distill`: rule drafting and lifecycle state.
- `internal/serve`: advisory serving snapshot behavior.
- `internal/demo`: deterministic demo fixture behavior.
- `internal/diffparse`: unified-diff touched-path parsing for assess and
  advise.
- `internal/assess`: assess command argument parsing, active memory rule
  serving projections, citation serving filters, repo-relative scope matching,
  and deterministic risk assessment construction for assess, serve, and advise.
- `internal/advise`: advisory comment decision, strategy, markdown rendering,
  confidence threshold handling, and prior-state comparison helpers.
- `internal/modelpull`: local model artifact pull argument parsing and
  manifest construction before config-owned validation and CLI-owned writes.
- `internal/distill`: distill argument parsing, provider plan construction,
  request-shape description, cost estimation, embedding-mode helpers, and
  distill review-gate helpers.
- `internal/review`: review command argument parsing, review action defaults,
  label validation, and edit scope-path validation before CLI-owned memory-rule
  lookup and artifact mutation.
- `internal/memory`: memory command argument parsing, default MEMORY.md output
  selection, format validation, and output path validation before CLI-owned
  memory-page rendering and artifact writes.
- `internal/serve`: serve command argument parsing, JSON format validation, and
  hosted/network transport dependency gating before CLI-owned rule loading and
  MCP manifest rendering.
- `internal/yamlmini`: minimal repo-local YAML parsing and line-reference
  inventory for config and memory rule documents.

## Required Promotion

- Source kind: review finding / architecture finding.
- Candidate mission: `systemic-architecture-budget`.
- Required command before implementation: `factoryd ingest --kind review` or
  the equivalent governed Factory planning path.
- Required validation after materialization: `make prepush-full`.
