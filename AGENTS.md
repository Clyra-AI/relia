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
  and missing-artifact behavior through the `model_artifact_pull` gate.
  Provider embeddings and LLM rule drafting are separate opt-in live
  capabilities that require the `model_provider_endpoint` gate with provider,
  model, endpoint or `base_url`, credential environment, budget posture,
  redaction posture, and allowlist; generic network or credential approval is
  not enough.

## 4. Required Boundaries

- cmd/relia/: CLI entrypoint.
- internal/: implementation packages.
- docs/product/: product requirements.
- docs/dev/: repo-local development guide.
- docs/architecture/: repo-local architecture guide.
- .factory/artifacts/: Factory planning, validation, and closure artifacts.
- Architecture budget: source files warn at 1200 lines and fail at 2500 lines
  by default. Existing Relia monolith debt in `cmd/relia/main.go` and
  `cmd/relia/main_test.go` is allowed only through
  `.factory/artifacts/exceptions/architecture-debt-relia-main.json`; tasks that
  touch those files must shrink them, split behavior into `internal/`, or keep
  the exception current with compensating validation.

## 5. Runner Readiness

Task packets must declare allowed_paths, forbidden_paths, validation_commands,
baseline_commands, red_first_commands, final_validation_commands,
acceptance_result_requirements, worker_type, factoryd_runtime,
evidence_required, worker_evidence_required, lifecycle_evidence_required,
lifecycle_gates, and stop_conditions before daemon dispatch.
`evidence_required` and `worker_evidence_required` are worker-owned evidence
that must exist before commit or PR shipping. `lifecycle_evidence_required` is
factoryd-owned evidence such as scope closure, PR lifecycle, and run-once
reports; workers must not fabricate those files or mark worker validation
blocked only because lifecycle artifacts do not exist before shipping.

When a task is run through Factory `autoship-supervisor`, bind supervision to
one task ID and keep the supervisor as the judgment layer only. It may classify
blockers, record explicit human acceptance, or make narrow repair after daemon
stop/non-convergence, but it must not replace `factoryd` as the implementation
or shipping engine. Supervisor decisions belong in
`.factory/artifacts/supervisor-runs/<task_id>/`.

## 6. CI And Scanner Posture

Relia is public. Pull requests must preserve the required-check manifest,
`validate`, and `CodeQL analyze` unless an approved scanner exception is
recorded with compensating validation evidence.

## 7. Post-PRD Finding Intake

Material `app-audit`, `repo-audit`, or `code-review` findings should be saved
as repo-local structured `finding-list` JSON before implementation. Use
`factoryd ingest --kind audit` for audit findings and
`factoryd ingest --kind review` for review findings so generated task packets
preserve the originating Factory skill refs, severity, evidence, affected
paths, minimum fix direction, and scope exclusions. Use Factory
`task-supervisor` when a human wants guided source-to-mission intake before
selecting an autoship task.

The MVP has one public release boundary: the final release/demo/product-signals
delivery slice. Do not clear that boundary when splitting release work into
runner-sized tasks.

## 8. Customer-Derived Learning

Do not commit raw customer code, logs, tickets, owner handles, endpoints,
credentials, screenshots, or machine-local paths. Customer-derived failures can
enter Relia only through a redacted intake record, an approved synthetic fixture,
or private delivery debt. Use `docs/dev/customer-failure-intake-template.md` and
`docs/architecture/lesson-record-template.md` before promoting a recurring
failure into reusable lessons or task packets.
