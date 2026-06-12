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
- Repository visibility: public.
- Factory mission: relia-mvp.
- Artifact namespace: Factory artifacts in .factory/artifacts/; daemon state in .factoryd/.
- Live credentials and network access are blocked until explicitly approved.
- Model artifacts are not bundled in source, binaries, containers, or releases.
  Local embeddings require an explicit `relia models pull` path with model ID,
  version, source, license, digest, cache path, update policy, rollback policy,
  and missing-artifact behavior. Provider endpoints remain separate opt-in live
  capabilities.

## 4. Required Boundaries

- cmd/relia/: CLI entrypoint.
- internal/: implementation packages.
- docs/product/: product requirements.
- docs/dev/: repo-local development guide.
- docs/architecture/: repo-local architecture guide.
- .factory/artifacts/: Factory planning, validation, and closure artifacts.

## 5. Runner Readiness

Task packets must declare allowed_paths, forbidden_paths, validation_commands, baseline_commands, red_first_commands, final_validation_commands, acceptance_result_requirements, worker_type, factoryd_runtime, evidence_required, lifecycle_gates, and stop_conditions before daemon dispatch.

## 6. CI And Scanner Posture

Relia is public. Pull requests must preserve the required-check manifest,
`validate`, and `CodeQL analyze` unless an approved scanner exception is
recorded with compensating validation evidence.

## 7. Post-PRD Finding Intake

Material `app-audit` or `code-review` findings must be saved as repo-local
markdown before implementation. Use `factoryd ingest --kind audit` for audit
findings and `factoryd ingest --kind review` for review findings so generated
task packets preserve the originating Factory skill refs.

The MVP has one public release boundary: the final release/demo/product-signals
delivery slice. Do not clear that boundary when splitting release work into
runner-sized tasks.
