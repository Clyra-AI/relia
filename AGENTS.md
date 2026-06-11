# AGENTS.md - Relia Repository Contract

Version: 0.1
Status: Normative
Scope: This repository only.

## 1. Scope And Intent

- Build Relia from [docs/product/prd.md](docs/product/prd.md).
- Treat the PRD as product truth and .factory/artifacts/ as governed delivery truth.
- Keep Factory artifacts under .factory/artifacts/ and transient daemon state under .factoryd/.

## 2. Required Validation

For normal changes, run:

- make lint-fast
- make test-fast
- make test-contracts

Before PR or merge, run:

- make prepush-full

## 3. Alignment Pins

- Runtime: Go 1.26.4.
- Module path: github.com/Clyra-AI/relia.
- Distribution target: standalone_binary.
- Factory mission: relia-mvp.
- Artifact namespace: Factory artifacts in .factory/artifacts/; daemon state in .factoryd/.
- Live credentials and network access are blocked until explicitly approved.

## 4. Required Boundaries

- cmd/relia/: CLI entrypoint.
- internal/: implementation packages.
- docs/product/: product requirements.
- docs/dev/: repo-local development guide.
- docs/architecture/: repo-local architecture guide.
- .factory/artifacts/: Factory planning, validation, and closure artifacts.

## 5. Runner Readiness

Task packets must declare allowed_paths, forbidden_paths, validation_commands, worker_type, factoryd_runtime, evidence_required, and stop_conditions before daemon dispatch.
