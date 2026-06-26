# Relia Architecture Guide

## Initial Boundaries

- CLI command surface
- Go module and command package layout
- configuration loading
- validation and evidence artifacts
- CI, CodeQL, and coverage feedback surfaces
- product-specific implementation packages under internal/

## CLI Result Boundary

The T1/T2 CLI skeleton owns the first public command-result and Phase 0
contract surface. Command state returns through `relia.command_result` JSON,
validated by `schemas/command-result.schema.json`, with examples in
`examples/command-results/exit-code-examples.json`. The envelope includes
`metadata` so downstream automation can bind output to the Relia version and
schema reference.

The state owner is the CLI package under `cmd/relia`; feedback sources are unit
tests, `make prepush-full`, Factory task-run evidence, and CI checks. The blast
radius is limited to local command output, `relia.yaml` bootstrap, schema
contracts under `schemas/`, local memory rule validation, and repo
operating-pack validation. Rollback is deletion of the T1/T2 schema/examples
and restoration of the previous command wrapper, but that would also remove the
agent-native CLI baseline required by
`docs/dev/dev_guides.md#agent-native-cli-policy`.

## Configuration And Artifact Boundary

`relia.yaml` is the repo-local source of truth for bootstrap configuration. T2
locks the MVP defaults to local-only privacy, private share scope, fail-closed
redaction, signature-only embeddings, no committed experience shards, advisory
serving, and disabled recurrence gates. `relia check` validates those defaults
offline and maps failures to stable command-model exit codes.

Generated stores live under `.relia/`; reviewed user-owned memory lives under
`memory/`; executable artifact schemas live under `schemas/`. `relia init`
creates the directory skeleton, but no generated experience, report, model, or
provider artifact is bundled in source. Local embedding refinement remains
blocked until `relia models pull` records the approved model manifest.

T3 makes `.relia/experiences/YYYY-MM.jsonl` the local generated store for
canonical experience records. The state owner remains `cmd/relia`; feedback
sources are ingest unit tests, command-result envelopes, `make prepush-full`,
and Factory task-run evidence. Redaction and entropy scanning run before shard
writes, and missing PR or URL provenance fails with exit `9`. Rollback is safe
deletion of generated `.relia/experiences` shards followed by a repeat
`relia ingest --input <path>` run, because the source of truth remains the
input event stream or future GitHub history.

T4.1 adds a committed, synthetic demo fixture boundary under `examples/demo/`
with customer-safe static baselines under `examples/reports/`. The state owner
is the fixture corpus plus the `cmd/relia` demo contract tests. Feedback sources
are `make test-contracts`, `make test-coverage`, and `make prepush-full`.
Deleting or editing `examples/demo/seeded-repo/prs.json`,
`examples/demo/seeded-repo/outcomes.jsonl`, or the report baselines changes the
first-session demo numbers and must preserve reproducibility: every summary
number is derived from the outcome stream, every citation resolves through the
seeded PR index, flake-discounted evidence does not draft a rule, and uncertain
attribution remains excluded. Rollback is deletion of the T4.1 fixture files
and the corresponding `TestDemo*` contract hook, which would also remove the
bundled demo baseline required by the PRD.

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
  bundled binary or repo payload. It requires an approved `model_artifact_pull`
  grant before any artifact download or refresh.
- Provider embeddings and LLM rule drafting are opt-in live provider paths, not
  substitutes for deterministic provenance. They require an approved
  `model_provider_endpoint` grant naming provider, model, endpoint or
  `base_url`, credential environment, budget posture, redaction posture, and
  allowlist before live provider calls.
- Missing local artifacts must fail closed for explicit local mode or be
  represented as signature-only provenance when signature mode is selected.

## Customer Failure And Lesson Boundary

Customer-derived learning is an evidence pipeline, not a global memory dump.
The architecture must preserve the distinction between private raw observations,
synthetic fixtures, reviewed lessons, and shipped product behavior.

- Raw customer material is not a Relia state source. It can only produce a
  redacted intake record, private delivery debt, or an approved synthetic
  fixture proposal.
- Synthetic fixtures are the preferred bridge from customer observation to
  testable behavior. They must keep expected outcome, provenance, and negative
  case information close to the test or example that consumes them.
- Lesson records must be narrow. A lesson needs applicability limits,
  non-applicability notes, expiry or revisit trigger, owner, and evidence refs.
- Broad rules without evidence, owner, applicability, and expiry are not valid
  architecture artifacts.
- Customer-failure intake and lesson records are post-MVP learning surfaces
  unless a human explicitly promotes them into PRD scope.

Use [lesson-record-template.md](lesson-record-template.md) for proposed memory
or lesson records before they are consumed by implementation tasks.
