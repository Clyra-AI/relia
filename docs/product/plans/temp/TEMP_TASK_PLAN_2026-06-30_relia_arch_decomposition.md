# TEMP Task Plan: Relia Architecture Budget And Decomposition

Date: 2026-06-30
Status: queued after Factory/factoryd architecture budget contracts
Repo: Relia

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
   task-scoped evidence required by the decomposition task.

## Candidate Package Boundaries

- `internal/result`: command-result envelope and helpers.
- `internal/config`: config loading and repo-relative path policy.
- `internal/ingest`: event ingestion, redaction, and provenance checks.
- `internal/backtest`: recurrence metrics and backtest report generation.
- `internal/distill`: rule drafting and lifecycle state.
- `internal/serve`: advisory serving snapshot behavior.
- `internal/demo`: deterministic demo fixture behavior.

## Validation

- `make lint-fast`
- `make test-fast`
- `make test-contracts`
- targeted command tests for moved behavior
- `make prepush-full`

## Acceptance Criteria

- `cmd/relia/main.go` becomes a thin command entrypoint.
- Product logic is no longer concentrated in one file.
- Existing CLI behavior and artifact outputs remain compatible.
- Future feature tasks cannot grow the old monolith without explicit
  architecture-debt approval.

