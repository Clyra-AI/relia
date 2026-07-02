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
- Memory-rule validation coverage now lives in
  `cmd/relia/memory_validation_test.go`, continuing to shrink
  `cmd/relia/main_test.go` without changing command behavior.
- Serve/assess command coverage and assessment rule validation coverage now
  live in `cmd/relia/assess_command_test.go` and
  `cmd/relia/assessment_rule_validation_test.go`, bringing
  `cmd/relia/main_test.go` below the fail threshold.
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
- `internal/distill` now owns distill clustering and canonical signature-key
  selection with direct package tests, while rule assembly and artifact writes
  remain in `cmd/relia`.
- `internal/distill` now also owns deterministic memory-rule ID and statement
  formatting with direct package tests, while rule assembly and artifact writes
  remain in `cmd/relia`.
- `internal/distill` now also owns memory-rule YAML rendering and scalar/float
  formatting with direct package tests, while existing lifecycle preservation
  and filesystem writes remain in `cmd/relia`.
- `internal/distill` now also owns distill evidence classification, flake
  discount handling, and avoid/playbook contradiction counting with direct
  package tests, while rule orchestration remains in `cmd/relia`.
- `internal/distill` now also owns distill confidence scoring and confidence
  metadata assembly with direct package tests, while rule orchestration remains
  in `cmd/relia`.
- `internal/distill` now also owns distill rule review labels, experience ID
  selection, and provenance reference assembly with direct package tests, while
  rule orchestration remains in `cmd/relia`.
- `internal/ingest` now also owns primary provenance URL selection for
  experience records, reusing its canonical GitHub URL helpers.
- `internal/distill` now also owns distill scope path/signal selection and
  drafted-rule summary metadata with direct package tests, while the CLI keeps
  filesystem writes and command-result assembly.
- `internal/review` now owns `relia review` argument parsing, repo-relative
  scope-path validation, memory rule lookup, lifecycle file updates, and
  review update validation with direct package tests, while command-result
  rendering remains in `cmd/relia`.
- `internal/memory` now owns `relia memory` argument parsing and
  repo-relative output-path validation with direct package tests, while memory
  rule summary loading, Markdown rendering, artifact writes, and command-result
  assembly remain in `cmd/relia`.
- `internal/memory` now also owns memory-rule artifact validation and drafted
  rule calibration checks with direct package tests, while CLI-specific
  CommandError construction remains injected by `cmd/relia`.
- `internal/memory` now also owns rule summary loading, provenance ordering,
  memory status counts, and MEMORY.md rendering with direct package tests,
  while `cmd/relia` keeps repo-relative output validation, artifact writes,
  and command-result assembly.
- `internal/serve` now owns `relia serve` argument parsing and hosted transport
  dependency gating with direct package tests, while rule loading, served-rule
  data assembly, and MCP manifest rendering remain in `cmd/relia`.
- `internal/assess` now owns `relia assess` argument parsing with direct
  package tests, while diff reads, rule loading, risk assessment invocation,
  and command-result rendering remain in `cmd/relia`.
- `internal/assess` now also owns active memory rule loading and active-rule
  validation with direct package tests, while CLI commands keep repository
  discovery, diff reads, command-result rendering, and error presentation.
- `internal/ingest` now owns `relia ingest` argument parsing with direct
  package tests, while input file reads, event normalization, shard writes, and
  command-result rendering remain in `cmd/relia`.
- `internal/backtest` now owns `relia backtest` argument parsing, recurrence
  window duration validation, recurrence report model, diagnostics/operator
  feedback/badge helpers, top repeated mistake aggregation, and HTML report
  rendering with direct package tests, while experience loading, baseline
  mutation, filesystem writes, and command-result rendering remain in
  `cmd/relia`.
- `internal/backtest` now also owns ERR baseline JSON comparison and freshly
  saved baseline status shaping with direct package tests, while repo-relative
  path validation and baseline/report file writes remain in `cmd/relia`.
- `internal/backtest` now also owns report repo ID derivation and ingest
  freshness metadata selection with direct package tests, while command-result
  metadata assembly remains in `cmd/relia`.
- `internal/backtest` now also owns recurrence report ID generation with direct
  package tests, while report assembly and filesystem persistence remain in
  `cmd/relia`.
- `internal/backtest` now also owns report windowing, recurrence metrics
  assembly, and report metadata assembly with direct package tests, while the
  CLI keeps recurrence orchestration, baseline mutation, filesystem
  persistence, and command-result rendering.

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
  validation, recurrence report model, report diagnostics/operator
  feedback/badge helpers, top repeated mistake aggregation, HTML report
  rendering, ERR baseline comparison, report repo ID derivation, and ingest
  freshness metadata selection, recurrence report ID generation, report
  windowing, recurrence metrics assembly, and report metadata assembly
  extracted; baseline/report file persistence and report generation
  orchestration remain candidates.
- `internal/distill`: rule drafting and lifecycle state.
- `internal/serve`: advisory serving snapshot behavior.
- `internal/demo`: deterministic demo fixture behavior.
- `internal/diffparse`: unified-diff touched-path parsing for assess and
  advise.
- `internal/assess`: assess command argument parsing, active memory rule
  loading and validation, active memory rule serving projections, citation
  serving filters, repo-relative scope matching, and deterministic risk
  assessment construction for assess, serve, and advise.
- `internal/advise`: advisory comment decision, strategy, markdown rendering,
  confidence threshold handling, and prior-state comparison helpers.
- `internal/modelpull`: local model artifact pull argument parsing and
  manifest construction before config-owned validation and CLI-owned writes.
- `internal/distill`: distill argument parsing, provider plan construction,
  request-shape description, cost estimation, embedding-mode helpers, distill
  clustering/review-gate helpers, deterministic memory-rule ID/statement
  formatting, memory-rule YAML rendering, evidence classification, flake
  discount handling, contradiction counting, confidence scoring, confidence
  metadata assembly, review-label selection, evidence ID selection, and rule
  provenance assembly, scope selection, status counts, and drafted-rule
  summary metadata.
- `internal/review`: review command argument parsing, review action defaults,
  label validation, edit scope-path validation, memory-rule lookup, lifecycle
  artifact mutation, and review update validation before CLI-owned
  command-result rendering.
- `internal/memory`: memory command argument parsing, default MEMORY.md output
  selection, format validation, output path validation, memory-rule artifact
  validation, drafted rule calibration checks, rule summary loading,
  provenance ordering, status counts, and MEMORY.md rendering before CLI-owned
  artifact writes.
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
