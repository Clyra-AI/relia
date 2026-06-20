# Decision 0001: CLI Command Result Envelope

Status: accepted
Date: 2026-06-20

## Context

T1 establishes the repo lifecycle baseline and the first agent-facing CLI
contract. The PRD requires stable exit codes and `--json`, and the Factory task
packet requires non-interactive machine-readable output plus quiet/compact
behavior that preserves status, evidence refs, typed exits, and errors.

## Decision

Relia commands return a `relia.command_result` envelope with schema version
`1.0`. Non-interactive stdout, `--json`, `--quiet`, and `--compact` emit JSON.
Interactive stdout emits concise human-readable text unless a machine-readable
flag is present. `relia init` and `relia check` are implemented for the T1
lifecycle baseline; later MVP commands are recognized and return typed
`not_implemented` envelopes until their task slices implement behavior.

The executable contract is:

- schema: `schemas/command-result.schema.json`
- exit examples: `examples/command-results/exit-code-examples.json`
- implementation: `cmd/relia/main.go`
- tests: `cmd/relia/main_test.go`

## Consequences

The CLI output surface is now a public contract before the rest of the MVP
commands are implemented. Future tasks that change envelope fields, exit-code
semantics, or default output mode must update the schema, examples, tests, and
developer guide in the same change.

Rollback is straightforward code and fixture deletion, but it would remove the
agent-native CLI baseline and should only happen with a replacement contract.
