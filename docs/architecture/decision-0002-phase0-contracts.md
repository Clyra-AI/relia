# Decision 0002: Phase 0 Schema And Artifact Contracts

## Status

Accepted

## Context

The PRD requires Relia to expose stable JSON output, versioned schemas,
versioned artifacts, local-private defaults, and fail-closed validation before
the ingest and distill implementation slices depend on those contracts.

## Decision

Relia uses schema version `1.0` for the Phase 0 source contracts under
`schemas/`. CLI artifact references include `schema_version`, and `relia check`
validates required schema files, config posture, basic memory-rule provenance,
unknown distill providers, and missing local embedding manifests for
`embeddings: local`.

Generated experience/report stores default to `.relia/`, reviewed memory
artifacts default to `memory/`, sharing defaults to `private`, and experience
commits default to `false`.

## Consequences

Automation can rely on stable schema paths and artifact ref versions before the
feature commands write real artifacts. The checks are intentionally preflight
level and standard-library only; later slices can add deeper artifact validation
without changing the baseline posture.

Rollback would restore the T1 command-result-only contract and remove the
schema/config checks, but that would leave PRD acceptance for versioned artifacts
and privacy defaults unimplemented.
