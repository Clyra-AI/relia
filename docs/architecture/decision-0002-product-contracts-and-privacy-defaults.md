# Decision 0002: Product Contracts And Privacy Defaults

Status: accepted
Date: 2026-06-21

## Context

T2 implements the PRD Phase 0 contract surface before ingest, distill, serve,
and PR advisory behavior are implemented. The PRD requires stable JSON output,
versioned schemas and artifacts, fail-closed redaction, local-only privacy
defaults, and explicit model-artifact or provider gates.

This task changes the pre-release command-result schema by adding a required
`metadata` object. It also makes `relia check` validate the checked-in schema
contracts and the default privacy posture in `relia.yaml`.

## Decision

Relia keeps command-result `schema_version` at `1.0` for the MVP pre-release
contract and adds required envelope metadata with `relia_version` and
`schema_id`. The executable contract remains:

- schema: `schemas/command-result.schema.json`
- examples: `examples/command-results/exit-code-examples.json`
- implementation: `cmd/relia/main.go`
- tests: `cmd/relia/main_test.go`

Phase 0 artifact contracts live in `schemas/` and must declare
`schema_version`, required fields, allowed enum values, forward-compatible
`metadata`, and `x-relia_error_mapping` for stable exits. `relia check`
validates the schema files before reporting the local operating pack as ready.
Outcome and failure-signature contracts use the PRD taxonomy names `revert`,
`merge_clean`, and `type_failure`. Memory-rule contracts use the documented
durable-rule shape with `object_type`, `schema_version`, `id`, `kind`, `status`,
`statement`, `confidence`, required `evidence`, `review`, scoped statements,
non-empty PR-backed `provenance`, and `metadata`; `relia check` fails with exit
`4` when rules lack schema-required fields, experience citations, provenance
entries, provenance outcomes, scope, metadata, or an accepted review label for
active rules. Experience record action blocks use the PRD canonical `pr` and
`commits` field names, and embedded signatures preserve signature class, check
name, key, message fingerprint, and extraction confidence so recurrence pairing
does not depend on a separate signature index being present. Experience,
coverage, and recurrence contracts use the PRD canonical repo string shape, and
recurrence report `error_recurrence_rate` remains a bounded `0` through `1`
proportion. Recurrence report headlines also require
`attribution_uncertain_count` and `flake_discounted_count` so trust-relevant
exclusions cannot disappear from JSON output. The risk-assessment contract
requires `matched_rules` with citations and `coverage_stats` so the shared
CLI/MCP/PR-advisor assessment surface can explain risky matches and OOD
coverage. The config contract admits the documented PR advisory and badge
sections, plus block or inline YAML sequences for config arrays. Version-only
PRD bootstrap configs are treated as `schema_version: "1.0"` and receive the
same MVP-safe privacy and artifact defaults before schema validation.

The default config remains deterministic and offline:

- `distill.embeddings: signature`
- `distill.review_required: true`
- `redaction.entropy_scan: true`
- `redaction.fail_closed: true`
- `memory.commit_experiences: false`
- `memory.share_scope: private`
- `memory.org_eligible: false`
- `serve.advisory_only: true`
- `advise.enabled: false`
- `badge.stale_after_days: 30`
- `badge.stale_after_merged_prs: 20`

Explicit `embeddings: local` fails closed with exit `8` until a later
`model_artifact_pull` flow records the required artifact metadata. Non-private
MVP sharing posture fails with exit `6`; unsafe redaction config fails with
exit `6`; invalid config/provider values fail with exit `2`; malformed memory
rules and active rules with missing or non-accepted review labels fail with exit
`4`.

## Consequences

The blast radius is limited to local CLI output, config validation, schema
contracts, and docs. Later behavior slices can add fields inside schema
`metadata` and command `data`, but changing stable envelope fields, privacy
defaults, schema compatibility, or live-work posture still requires docs,
tests, and an architecture decision.

Rollback would remove T2 schemas, config validation, and metadata from command
results. That rollback would also remove the PRD Phase 0 entry gate for later
ingest, distill, serve, and advisory tasks.
