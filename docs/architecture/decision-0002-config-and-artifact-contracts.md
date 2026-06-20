# Decision 0002: Config And Artifact Contracts

Status: accepted
Date: 2026-06-20

## Context

T2 implements the PRD requirement for versioned schemas and artifacts, privacy
defaults, validation, and structured output guarantees. The repository must stay
offline by default, avoid ambient credentials, and keep generated memory local
unless a later explicit export or provider gate is approved.

## Decision

`relia.yaml` is the local configuration contract with `schema_version: "1.0"`
and `artifact_contract_version: "1.0"`. `relia check` validates the config,
required schema files, schema versions, artifact contract refs, and privacy
defaults. Unsafe privacy changes fail closed with a typed
`configuration_validation_failed` error and `redaction_status: failed_closed`.

The executable contracts are:

- config schema: `schemas/relia-config.schema.json`
- command result schema: `schemas/command-result.schema.json`
- experience record schema: `schemas/experience-record.schema.json`
- memory rule schema: `schemas/memory-rule.schema.json`
- local memory artifact root: `memory/`

Default posture remains offline: no network, no ambient credentials, signature
embeddings only, provider mode disabled, local model artifacts requiring an
explicit pull path, advisory-only serving, and gate enforcement disabled.

## Consequences

Config and memory artifacts are now public pre-1.0 contracts. Future changes to
field names, schema versions, artifact paths, privacy defaults, or output refs
must update config, schemas, examples, tests, README, and developer guidance in
the same task. Rollback is possible by deleting the T2 schema/config additions,
but it would remove the PRD's artifact-stability and privacy validation
baseline.
