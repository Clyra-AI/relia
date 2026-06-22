# Decision 0002: Phase 0 Configuration And Artifact Contracts

Status: accepted
Date: 2026-06-22

## Context

T2 implements the PRD Phase 0 contract layer before ingest, distill, serve, or
advisory behavior exists. The task changes public CLI JSON output by adding
`metadata`, expands `relia.yaml`, and introduces versioned artifact schemas.
The repo policy requires a decision note for public output and schema contract
changes.

## Decision

Relia keeps the command-result schema at `1.0` and adds required envelope
`metadata` with Relia version, schema reference, and schema version. This is a
compatible pre-1.0 contract extension and is reflected in
`schemas/command-result.schema.json` plus exit-code examples.

`relia.yaml` now makes the MVP defaults explicit:

- local-only privacy and private share scope
- no outbound code, diff, log, or experience-record sharing by default
- fail-closed redaction with entropy scanning and standard token shapes
- signature-only embeddings unless an approved local model artifact or provider
  endpoint gate is configured
- no committed experience shards by default
- advisory-only serving and disabled recurrence gate

Phase 0 schemas live under `schemas/` and each requires `schema_version` plus
forward-compatible `metadata`. `relia check` validates these contracts offline
and maps failures to the stable command-model exit codes.

## Consequences

Automation consumers can rely on a versioned envelope and schema references
before later MVP commands are implemented. Local bootstrap remains deterministic
and does not require credentials, network, provider endpoints, or model
artifacts.

The validation is intentionally contract-level. It does not implement live
GitHub checks, ingestion, distillation, MCP serving, or provider calls. Future
tasks may deepen schema validation, but they must preserve the stable exit-code
mapping or update this decision and the executable examples together.
