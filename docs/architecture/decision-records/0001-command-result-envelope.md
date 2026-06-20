# 0001 Command Result Envelope

Status: Accepted
Date: 2026-06-20

## Context

T1 establishes the repository lifecycle foundation and the first agent-facing
CLI contract. The PRD requires stable exit codes, `--json`, machine-readable
non-interactive output, and quiet/compact modes that preserve status, evidence
refs, typed exits, and errors.

## Decision

Relia command output uses `relia.command_result` schema version `1.0`.
Non-interactive stdout, `--json`, `--quiet`, and `--compact` emit JSON.
`--quiet` and `--compact` use compact JSON rather than suppressing the
envelope.

Unknown commands return exit `2` with `invalid_usage`. Recognized MVP commands
that are not implemented in this foundation slice return exit `1` with
`not_implemented` and PRD evidence refs. Stable examples for exit codes `0`
through `9` live in `examples/command-results/`, and the executable schema lives
in `schemas/command-result.schema.json`.

## Systems Map

- State owner: `cmd/relia`, `schemas/command-result.schema.json`, and
  `examples/command-results/`.
- Feedback source: `make test-fast`, `make test-coverage`,
  `make test-contracts`, and `make prepush-full`.
- Blast radius: CLI stdout/stderr, automation callers, tests, docs, and future
  schema-compatible command implementations.
- Source-of-truth impact: aligns the executable CLI contract with
  `docs/product/prd.md#command-model` and
  `docs/dev/dev_guides.md#agent-native-cli-policy`.
- Rollback/deletion checks: do not delete or change the schema, examples, or
  envelope fields without updating tests, docs, and a replacement decision
  record.

## Consequences

Future command implementations must keep the `1.0` envelope compatible or
introduce an explicit schema-version migration. The foundation slice does not
perform network, credential, model, or product-data work.
