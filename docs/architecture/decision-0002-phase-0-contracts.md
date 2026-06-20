# Decision 0002: Phase 0 Schema And Config Contracts

## Status

Accepted for MVP Phase 0.

## Context

The PRD requires stable CLI output, versioned schemas and artifacts, local
privacy defaults, and fail-closed redaction behavior before ingest and distill
implementation begins.

## Decision

Relia stores executable product schemas under `schemas/` with schema version
`1.0`, object discriminators, required fields, enum values, and
forward-compatible `metadata` objects. `relia.yaml` carries local-first privacy
defaults: signature embeddings, no committed experience cache, entropy scanning,
review required before active rules, advisory-only serving, and a disabled
recurrence gate.

`relia check` validates the schema inventory, tracked artifact skeleton, and
privacy defaults. It maps local config errors to exit `2`, schema or artifact
contract failures to exit `4`, and unsafe redaction defaults to exit `6`.

## Consequences

Downstream tasks can build ingest, distill, memory, and assess behavior against
stable artifact names and typed validation exits. The first implementation uses
standard-library JSON parsing for executable schemas and conservative
line-oriented checks for `relia.yaml` defaults to avoid adding an offline YAML
dependency during bootstrap.
