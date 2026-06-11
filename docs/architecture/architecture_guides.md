# Relia Architecture Guide

## Initial Boundaries

- CLI command surface
- Go module and command package layout
- configuration loading
- validation and evidence artifacts
- product-specific implementation packages under internal/

## Systems Thinking Map

- State lives in repo-local config, generated artifacts, source files, and Factory evidence.
- Source of truth for product scope is docs/product/prd.md.
- Source of truth for governed delivery state is .factory/artifacts/prd-to-plan/relia-mvp/.
- Feedback lives in command output, tests, validation reports, PR lifecycle reports, and scope closure.
- Deleting Factory artifacts breaks governed closure; deleting dev/architecture guides breaks task propagation.

## TDD And Red-First Expectations

- Behavior changes should add or update a failing test, fixture, schema example, or validator expectation before implementation when practical.
- If test-first is not practical, the validation report must carry a structured skipped reason.

## ADR And Decision Triggers

Require a decision note when a task changes runtime pins, distribution target, credential/network posture, public output contracts, schema compatibility, or major reliability/performance tradeoffs.

## Trust-Mode Posture

- Deterministic bootstrap has no network, no ambient secrets, and no live credentials.
- Approved live work must use explicit config and record credential/network posture in evidence.
