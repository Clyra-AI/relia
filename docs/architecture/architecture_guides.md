# Relia Architecture Guide

## Initial Boundaries

- CLI command surface
- Go module and command package layout
- configuration loading
- validation and evidence artifacts
- CI, CodeQL, and coverage feedback surfaces
- product-specific implementation packages under internal/

## Systems Thinking Map

- State lives in repo-local config, generated artifacts, source files, and Factory evidence.
- Source of truth for product scope is docs/product/prd.md.
- Source of truth for governed delivery state is .factory/artifacts/prd-to-plan/relia-mvp/.
- Feedback lives in command output, tests, coverage gates, CodeQL status, validation reports, PR lifecycle reports, and scope closure.
- Deleting Factory artifacts breaks governed closure; deleting dev/architecture guides breaks task propagation.
- Deleting required-check metadata, CODEOWNERS, action-ref exceptions, or CI workflows breaks public-repo delivery controls.

## TDD And Red-First Expectations

- Behavior changes should add or update a failing test, fixture, schema example, or validator expectation before implementation when practical.
- If test-first is not practical, the validation report must carry a structured skipped reason.

## ADR And Decision Triggers

Require a decision note when a task changes runtime pins, distribution target, credential/network posture, public output contracts, schema compatibility, or major reliability/performance tradeoffs.

Model artifact and inference-runtime decisions require an ADR before
implementation when they affect local embeddings, packaging, release artifacts,
cross-platform support, memory/CPU cost, or dependency footprint. The ADR must
choose the inference boundary, such as pure Go library, ONNX/runtime binding, or
external local process, and record license, size, security, update, rollback,
and cache-location implications.

## Trust-Mode Posture

- Deterministic bootstrap has no network, no ambient secrets, and no live credentials.
- Approved live work must use explicit config and record credential/network posture in evidence.

## Model Artifact Boundary

- Deterministic signature clustering is the zero-install trust anchor.
- Local embedding refinement is an explicitly pulled model-artifact path, not a
  bundled binary or repo payload.
- Provider embeddings and LLM rule drafting are opt-in live provider paths, not
  substitutes for deterministic provenance.
- Missing local artifacts must fail closed for explicit local mode or be
  represented as signature-only provenance when signature mode is selected.
