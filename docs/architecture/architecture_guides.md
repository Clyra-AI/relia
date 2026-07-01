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

T4.2 adds committed, synthetic distill/review lifecycle fixture expectations in
`examples/demo/distill-review-lifecycle-fixtures.json`. The state owner remains
the fixture corpus plus the `cmd/relia` demo contract tests. Feedback sources
are `make test-contracts`, `make test-coverage`, and `make prepush-full`. The
fixture records recurrence draft, contradicted, and stale outcomes with
repo-relative evidence refs, seeded PR citations, and a compiled serving
snapshot that excludes non-active rules. Rollback is deletion of the lifecycle
fixture, the synthetic lifecycle PR/outcome rows, and the corresponding
`TestDemoDistillReviewLifecycleFixtures` hook, which would remove acceptance
coverage for planted contradiction and stale-path behavior.

T4.3 adds committed, synthetic assessment fixtures under
`examples/demo/assessment-fixtures/` plus one active demo memory rule in
`memory/rules/demo-assessment-active-rule.yaml`. The state owner remains the
fixture corpus, the local `relia assess` command surface in `cmd/relia`, and
the `cmd/relia` demo contract tests. Feedback sources are
`make test-contracts`, `make test-coverage`, and `make prepush-full`. The
fixture records one planted-pattern diff that must return `match_high` with
seeded PR citations and one unknown-path diff that must return `no_coverage`.
Rollback is deletion of the assessment fixture directory, the demo memory rule,
the bounded `assess` command slice, and the corresponding
`TestDemoAssessmentFixturesDriveAssessCommand` hook, which would remove
acceptance coverage for assessment behavior.

T5 adds the local `relia backtest` command surface and the recurrence report
contract in `schemas/recurrence-report.schema.json`. The state owner is
`cmd/relia` plus local generated artifacts under `.relia/experiences`,
`.relia/reports`, and `.relia/baselines`. Feedback sources are unit tests,
schema contract validation, `make test-contracts`, `make test-coverage`, and
Factory task-run evidence. The command is offline and deterministic: it uses
the latest local experience timestamp as the window anchor, computes confirmed
and possible recurrence pairs, excludes possible pairs and discounted flakes
from headline ERR, compares any saved baseline by source artifact digest, and
keeps the recurrence gate disabled by default. The blast radius is limited to
local command output, generated report files, baseline comparison, attribution
config validation, and the recurrence-report schema. Rollback is removal of the
`backtest` command implementation, T5 tests, docs, and schema expansion; local
`.relia/reports` and `.relia/baselines` files can be deleted because experience
shards remain the source of truth.

T6 adds the local `relia distill`, `relia review`, and `relia memory` command
surfaces. The state owner is `cmd/relia`, local generated experience shards
under `.relia/experiences`, reviewed memory artifacts under `memory/rules`, and
the rendered `memory/MEMORY.md` page. Feedback sources are unit tests, schema
contract validation, `make test-contracts`, `make test-coverage`, and Factory
task-run evidence. The command path is offline and deterministic by default:
explicit `--input` files use the same redacted local outcome normalization as
ingest without persisting shards, canonical signature keys cluster records
before signature ID fallback, provider drafting fails closed without an approved
model-provider gate, and confidence is calculated from evidence count, the PRD
default 90-day recency half-life, contradictions, flake discounts, and
extraction confidence with no drafting-model contribution. Drafted
`cluster_summary` and future `llm_drafted` rules are invalid unless their
metadata carries confidence inputs and decay fields. The blast radius is limited
to generated memory-rule YAML, review status transitions, memory-page
rendering, and assessment serving eligibility because only accepted active rules
are served.
Rollback is removal of the T6 command implementation, tests, docs, and generated
`memory/rules/*.yaml` or `memory/MEMORY.md`; `.relia/experiences` remains the
source of truth for re-distillation.

T7 completes the deterministic distillation boundary without adding live model
or network behavior. The state owner remains `cmd/relia`, with generated rule
artifacts under `memory/rules`, rendered memory under `memory/MEMORY.md`, and
local model manifests under `.relia/models/manifest.json` when an operator
records an already-present local artifact. Feedback sources are focused
distill/review/model tests, schema contract validation, `make prepush-full`,
and Factory task-run evidence. Deterministic clustering now uses canonical
signature keys before signature ID fallback, confidence is capped at `0.6`
until three confirmed experiences exist, review supports approve/edit/reject
state transitions, and MEMORY.md separates strong active memory from weak
candidate, stale, contradicted, and retired memory. The local embedding
inference boundary is decision-recorded in
`docs/architecture/decision-0003-local-embedding-boundary.md`: T7 validates
manifests and fails closed for missing, stale, or digest-mismatched artifacts,
but does not add model weights, inference runtime payloads, or runtime
embedding refinement.
Rollback is removal of the T7 command refinements, tests, docs, and any local
`.relia/models/manifest.json`; signature-only distillation remains the fallback
source of truth.

T8 adds report/evidence feedback without changing the local trust posture. The
state owner remains `cmd/relia`, with generated recurrence reports under
`.relia/reports`, generated rules under `memory/rules`, rendered memory under
`memory/MEMORY.md`, and Factory task-run evidence under `.factory/artifacts/`.
Feedback sources are unit tests, schema contract validation,
`make prepush-full`, and task-run evidence. Backtest reports now carry
operator-visible metrics, top repeated mistakes, diagnostics, summary text, and
badge-staleness fields derived from canonical experience records, explicit
`last_ingest_at` metadata, and merged-PR activity metadata, without adding
network access to the offline backtest command. Ingest and canonical experience
consumers reject inputs marked as agent self-reports or reflections before
persistence or memory-rule writes, and
generated rules record `metadata.memory_source: verified_outcome_events`. The
blast radius is limited to local CLI output, recurrence-report schema and
artifacts, generated rule metadata, and docs. Rollback is removal of the T8
report fields, tests, docs, and generated `.relia/reports` or `memory/rules`
files; canonical
experience shards remain the source of truth for regeneration.

T9 adds provider, advisory, and local serve integration boundaries while
preserving the offline trust posture. The state owner remains `cmd/relia`, with
provider plans in command-result JSON, advisory planner artifacts under
`.relia/reports`, active rules under `memory/rules`, and the optional advisory
workflow under `.github/workflows/relia-advisory.yml`. Feedback sources are
provider/advisory unit tests, workflow metadata, `make prepush-full`, and
Factory task-run evidence. Provider-backed distill validates OpenAI-compatible
and Anthropic adapter config, estimates tokens and cost from redacted local
records, enforces `distill.max_cost_usd_per_run`, and fails closed before
network or credential use without a complete `model_provider_endpoint` grant;
provider base URLs with user-info or embedded credentials are rejected before
they can be echoed in provider plans.
`relia serve --format json` exposes a local MCP capability manifest for
`recall`, `assess`, and `coverage` over active accepted rules only; hosted or
network transports fail closed. `relia advise` reuses the assessment engine to
write one-comment advisory artifacts while respecting disabled advise config,
unchanged diff fingerprints, and a zero comment cap. The GitHub Action uses the
explicit Actions token only in GitHub API steps, disables persisted checkout
credentials, runs trusted base-branch Relia code and memory rules against the
PR diff under `pull_request_target`, seeds local state from bot-authored
hidden-marker PR comment pages, carries marker timestamps to enforce the
configured reassessment debounce window, parses planner JSON directly during
publish without sourcing planner-produced shell environment text, and never
grants the token to Relia code. Existing advisory comments are updated with a
cleared state when a later diff becomes covered clean. The blast radius is limited to CLI output, local
`.relia/reports` advisory artifacts, provider config validation, and optional
PR comments.
Rollback is removal of the T9 command paths, workflow, docs, and generated
`.relia/reports/advisory-*` files; repository memory artifacts remain portable
and useful without any hosted Relia account.

## Systems Thinking Map

- State lives in repo-local config, generated artifacts, source files, and Factory evidence.
- Source of truth for product scope is docs/product/prd.md.
- Source of truth for governed delivery state is .factory/artifacts/prd-to-plan/relia-mvp/.
- Feedback lives in command output, tests, coverage gates, CodeQL status, validation reports, PR lifecycle reports, and scope closure.
- Deleting Factory artifacts breaks governed closure; deleting dev/architecture guides breaks task propagation.
- Deleting required-check metadata, CODEOWNERS, action-ref exceptions, or CI workflows breaks public-repo delivery controls.

## Architecture Budget And Decomposition

Relia follows the Factory architecture budget gate: source files warn at `1200`
lines and fail at `2500` lines. The inventory excludes daemon state,
dependencies, caches, and build output, but it does include product source and
tests. The only approved over-budget source surfaces are `cmd/relia/main.go`
and `cmd/relia/main_test.go`, recorded in
`.factory/artifacts/exceptions/architecture-debt-relia-main.json` and backed by
`docs/architecture/findings/TEMP_FINDING_2026-06-30_relia_arch_decomposition.md`.

Until that exception is closed, product work that touches `cmd/relia/main.go`
or `cmd/relia/main_test.go` must either reduce size, move coherent behavior
into `internal/`, or record why the change is shrink-neutral with compensating
validation. New feature work must not add fresh product domains to the CLI
entrypoint.

Decomposition starts with cohesive pure behavior that can move without changing
the CLI contract. `internal/diffparse` owns unified-diff touched-path parsing
for `assess` and `advise`; `internal/yamlmini` owns Relia's minimal
repo-local YAML parsing and line-reference inventory for config and memory
rule documents; `internal/config` owns default config document rendering,
config document references, config loading and validation, provider/advisory
configuration parsing, and local model-manifest dependency validation;
`internal/result` owns the command-result envelope and generic pass/error
constructors; `internal/ingest` owns ingest input parsing, fail-closed
redaction, and standard secret/provenance URL token-shape checks.
`cmd/relia` keeps command wiring, command-specific CommandError translation,
experience-record normalization, persistence coordination, and human/JSON
rendering for those surfaces.

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
