# Relia Architecture Guide

## Initial Boundaries

- CLI command surface
- Go module and command package layout
- configuration loading
- validation and evidence artifacts
- CI, CodeQL, and coverage feedback surfaces
- product-specific implementation packages under internal/

## CLI Result Boundary

The T1 CLI skeleton owns the first public command-result contract. Command state
returns through `relia.command_result` JSON, validated by
`schemas/command-result.schema.json`, with examples in
`examples/command-results/exit-code-examples.json`. The state owner is the CLI
package under `cmd/relia`; feedback sources are unit tests, `make
prepush-full`, Factory task-run evidence, and CI checks. The blast radius is
limited to local command output, `relia.yaml` bootstrap, and repo operating-pack
validation. Rollback is deletion of the T1 schema/examples and restoration of
the previous command wrapper, but that would also remove the agent-native CLI
baseline required by `docs/dev/dev_guides.md#agent-native-cli-policy`.

## Config And Artifact Contract Boundary

T2 makes `relia.yaml` a validated source of truth for local configuration,
privacy defaults, and artifact contracts. `relia check` owns validation for the
config file, schema refs, and repo-relative memory artifact roots. The state
owner is the CLI package under `cmd/relia`; the durable contract files are
`relia.yaml`, `schemas/relia-config.schema.json`,
`schemas/experience-record.schema.json`, `schemas/memory-rule.schema.json`, and
`memory/README.md`. Feedback sources are unit tests, `make prepush-full`, and
Factory task-run evidence. The blast radius is local bootstrap and structured
output only; no live provider, credential, network, model artifact, or release
packaging posture changes in this boundary.

Rollback is deletion of the T2 schemas/config fields plus restoration of the
T1-only `relia check`, but doing so would remove executable validation for the
PRD's schema, artifact-stability, privacy, and structured-output requirements.

## Systems Thinking Map

- State lives in repo-local config, generated artifacts, source files, and Factory evidence.
- Source of truth for product scope is docs/product/prd.md.
- Source of truth for governed delivery state is .factory/artifacts/prd-to-plan/relia-mvp/.
- `relia.yaml` owns local bootstrap defaults; schema files under `schemas/` own
  structured contracts; `memory/` is the reserved local memory artifact root.
- Feedback lives in command output, tests, coverage gates, CodeQL status, validation reports, PR lifecycle reports, and scope closure.
- Deleting Factory artifacts breaks governed closure; deleting dev/architecture guides breaks task propagation.
- Deleting `relia.yaml`, schema files, or `memory/README.md` breaks local
  contract validation and should fail `relia check`.
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
