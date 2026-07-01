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

## Candidate Package Boundaries

- `internal/result`: command-result envelope and helpers.
- `internal/config`: default config rendering, config document references,
  config loading and validation, provider/advisory configuration parsing,
  local model-manifest dependency validation, and repo-relative path policy.
- `internal/ingest`: event ingestion, redaction, and provenance checks.
- `internal/backtest`: recurrence metrics and backtest report generation.
- `internal/distill`: rule drafting and lifecycle state.
- `internal/serve`: advisory serving snapshot behavior.
- `internal/demo`: deterministic demo fixture behavior.
- `internal/diffparse`: unified-diff touched-path parsing for assess and
  advise.
- `internal/yamlmini`: minimal repo-local YAML parsing and line-reference
  inventory for config and memory rule documents.

## Required Promotion

- Source kind: review finding / architecture finding.
- Candidate mission: `systemic-architecture-budget`.
- Required command before implementation: `factoryd ingest --kind review` or
  the equivalent governed Factory planning path.
- Required validation after materialization: `make prepush-full`.
