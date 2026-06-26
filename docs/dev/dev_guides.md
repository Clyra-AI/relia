# Relia Developer Guide

## Toolchain Pins

| Tool | Version |
|---|---:|
| Go | 1.26.4 |

The Makefile sets `GOCACHE` under `TMPDIR` by default so sandboxed validation
does not depend on a writable user-level Go build cache.

## Validation Matrix

- make lint-fast: repo operating pack and layout checks.
- make test-fast: Go unit tests.
- make test-coverage: Go coverage gate over first-party CLI packages.
- make test-contracts: Factory planning artifact and repo-pack checks.
- make prepush-full: full local gate before PR or merge.

## 12-Level Test Matrix

| Tier | Status | Current command, check, or evidence |
|---|---|---|
| Tier 1 Unit | Active | go test ./... |
| Tier 2 Integration | Planned | make test-contracts |
| Tier 3 End-to-End | Planned | CLI invocation tests as product commands mature |
| Tier 4 Acceptance | Planned | .factory/artifacts/prd-to-plan/relia-mvp/scope-closure-map.json |
| Tier 5 Hardening | Planned | fail-closed and recovery tests |
| Tier 6 Chaos | Reserved | future controlled failure injection |
| Tier 7 Performance | Reserved | future runtime/cost budgets |
| Tier 8 Soak | Reserved | future repeated-run stability |
| Tier 9 Contract | Active | make test-contracts |
| Tier 10 UAT | Reserved | distribution acceptance |
| Tier 11 Scenario | Planned | PRD scenario coverage |
| Tier 12 Cross-System Integration | Blocked until approved | live/network/credential checks |

Future task packets must cite applicable tiers or record an approved non-applicable reason.

## Coverage Gates

Relia inherits the org-wide Factory coverage policy derived from Wrkr's launch
standard.

| Scope | Minimum | Enforcement |
|---|---:|---|
| Go first-party packages overall (`cmd/`) | `>= 75%` | `make test-coverage`, included in `make prepush-full` and CI |

Coverage output is written to `.factory/tmp/coverage.out`. A task that changes
runtime behavior, schemas, CLI output, or package boundaries must cite
`coverage_policy_refs` or record an approved coverage exception with
compensating validation evidence.

## CI And PR Lifecycle

- GitHub Actions workflow: .github/workflows/validate.yml.
- Required local command: make prepush-full.
- Security scanner: required CodeQL for Go via .github/workflows/codeql.yml.
- Required-check manifest: .github/required-checks.json, expected checks validate and CodeQL analyze.
- Workflow hardening: validate and CodeQL workflows declare least-privilege permissions, concurrency cancellation, job timeouts, and toolchain setup from pinned repo files.
- PR lifecycle report path: .factory/artifacts/pr-lifecycle/<work_item_id>/pr-lifecycle-report.json.

## Bootstrap Rules

- Deterministic bootstrap must not require network, sandbox credentials, or model keys.
- Evidence artifacts must use repo-relative paths.
- New dependencies must be pinned and justified.

## Outcome Fixture Corpus

T4 is intentionally split into small runner-sized corpus tasks:

- T4.1 builds seeded outcome fixtures, flake-discount fixtures, redaction
  fixtures, attribution precision samples, and reproducible demo-number
  baselines.
- T4.2 builds distill/review lifecycle fixtures for recurrence,
  contradiction, and stale-path behavior.
- T4.3 builds assessment fixtures for planted-pattern and unknown-path diffs.

Do not collapse these back into one broad corpus task. Each task must add the
smallest fixture set needed for its acceptance items and must keep fixture
provenance, expected outcomes, and negative cases close to the tests that use
them.

T4.1 fixture ownership:

- Seeded PR index: `examples/demo/seeded-repo/prs.json`.
- Seeded outcome stream: `examples/demo/seeded-repo/outcomes.jsonl`.
- Attribution precision sample:
  `examples/demo/attribution-precision-sample.json`.
- Flake-discount fixture: `examples/demo/flake-discount-fixtures.json`.
- Redaction fixture:
  `examples/demo/redaction-fixtures/expected-redacted-artifacts.json`.
- Static report baselines: `examples/reports/backtest-demo.json`,
  `examples/reports/backtest-demo.html`, and
  `examples/reports/memory-page-demo.md`.

T4.2 fixture ownership:

- Distill/review lifecycle expectations:
  `examples/demo/distill-review-lifecycle-fixtures.json`.
- Lifecycle citation anchors remain in
  `examples/demo/seeded-repo/prs.json`.
- Lifecycle outcomes that should not change the demo ERR denominator remain in
  `examples/demo/seeded-repo/outcomes.jsonl` as clean merges.

The contract lane runs `TestDemo*` fixture contracts in `cmd/relia` after the
repo-pack validator. Those tests recompute the demo ERR headline from the
seeded outcome stream, resolve every report citation through the seeded PR
index, verify the planted flaky test is discounted and drafts no rule, enforce
attribution precision with uncertain cases excluded, validate distill/review
lifecycle fixtures for recurrence draft, contradicted, and stale serving
outcomes, and scan demo artifacts for standard secret token shapes.

## Customer Failure Intake

Customer-derived failures may improve Relia only through a bounded,
redacted, reviewable intake loop. Raw customer code, private logs, tickets,
screenshots, owner handles, endpoints, credentials, and machine-local paths
must not be committed.

Use [customer-failure-intake-template.md](customer-failure-intake-template.md)
when a support case, customer repo, or downstream audit suggests a reusable
lesson. The intake record must include:

- the observed failure mode, stated without customer identifiers
- the affected Relia behavior or acceptance item
- redaction evidence and reviewer approval
- a synthetic fixture proposal or reason no fixture is safe
- applicability limits and expiry/revisit trigger
- owner, evidence refs, and promotion decision

Promotion into examples, tests, fixtures, or memory content requires owner
approval and a synthetic or public-safe fixture. If safe reproduction is not
possible, keep the item as private delivery debt or an approved deferral; do
not generalize it into product behavior.

## Agent-Native CLI Policy

- Agent-facing commands must support stable JSON output mode before they become
  automation surfaces.
- Commands should emit machine-readable output when stdout is not a TTY unless
  explicitly human-only with an approved exception.
- `--quiet` and `--compact` must preserve status, evidence refs, typed errors,
  and the command-result envelope.
- Task packets that touch CLI behavior must carry acceptance checks for JSON
  mode, piped or non-interactive behavior, quiet/compact posture, typed exits,
  and machine-readable errors.

T1 establishes the command result envelope as the baseline automation contract:

- envelope schema: `schemas/command-result.schema.json`
- examples for exit codes `0` through `9`:
  `examples/command-results/exit-code-examples.json`
- implemented lifecycle commands: `relia init`, `relia check`, `relia help`,
  and `relia version`
- primary MVP commands not implemented in T1 return a typed
  `not_implemented` error envelope rather than an ambiguous parser failure

The envelope preserves `object_type`, `schema_version`, `command`, `status`,
`mode`, `exit_code`, `warnings`, `errors`, `artifacts`, `evidence_refs`,
`duration_ms`, `redaction_status`, and `metadata`. Human-readable output is only
the default when stdout is an interactive terminal and no machine-readable flag
is present.

T2 extends the contract layer:

- `relia init` writes the artifact skeleton for `.relia/experiences`,
  `.relia/signatures`, `.relia/coverage`, `.relia/reports`,
  `.relia/baselines`, `memory/rules`, and `memory/compiled`.
- `relia check` validates the explicit local-only config defaults in
  `relia.yaml`, required Phase 0 schema files, and memory rule provenance when
  rule artifacts exist.
- Unsafe privacy or redaction settings fail closed with the stable exit code
  assigned by the command model.
- `distill.embeddings: local` fails closed with exit `8` until an approved
  model-artifact pull writes the configured local manifest.

T3 adds offline ingestion:

- `relia ingest --input <path>` accepts local JSON, JSON arrays, `{"events":[]}`
  objects, or JSONL streams of outcome events.
- Input is recursively redacted and entropy-scanned before any artifact is
  opened for write. Known token shapes and secret-named fields are redacted;
  unclassified high-entropy values fail closed with exit `6`.
- Normalized records use `schemas/experience-record.schema.json` and are
  idempotently upserted into monthly `.relia/experiences/YYYY-MM.jsonl` shards.
- Each persisted experience requires a PR number and at least one
  `https://github.com/` provenance URL. Missing or invalid provenance fails with
  exit `9`.
- `attribution.uncertain: exclude` remains the default; uncertain events are
  skipped instead of persisted, while explicit human and agent outcomes can both
  become rule evidence.

## Structured Data, Proof, Budgets, And Redaction

- PR, check-run, experience, memory, MCP, assessment, report, and config data
  must be handled through parsers, schemas, or stable APIs.
- Phase 0 schemas are required for experience records, outcome evidence,
  failure signatures, memory rules, coverage maps, risk assessments, recurrence
  reports, compiled context, command results, and redaction config. Every schema
  must require `schema_version` and forward-compatible `metadata`.
- Structured check-run data is preferred over log parsing; any log-parsed
  fallback must be labeled and validated.
- Task closure must name the required proof level: syntax, source evidence,
  workflow behavior, or user-visible behavior. Workflow or user-visible closure
  requires proof-of-behavior scorecard evidence or an approved exception.
- Large logs, reports, traces, and generated evidence must be cited by artifact
  ref with full-output hashes and truncation metadata instead of duplicated
  payloads.
- Customer-safe or public artifacts must recursively redact nested owner,
  credential, endpoint, secret, and machine-local path fields.

## Model Provider And Artifact Policy

- Default clustering must run with `embeddings: signature` and require no
  network, provider credential, model key, or downloaded model artifact.
- `embeddings: local` requires an explicit `relia models pull` command before
  use. The pulled artifact must record model ID, version, source URL, license,
  SHA-256 content digest, cache path, update policy, and rollback policy.
- If `embeddings: local` is configured and the artifact is absent, stale, or
  digest-mismatched, Relia must fail closed with exit `8` and clear remediation.
  It must not silently degrade to provider embeddings or unlabeled signature-only
  behavior.
- `embeddings: provider` is a separate live provider capability. Provider
  endpoint work must declare provider, model, endpoint or `base_url`, credential
  environment, cost cap, redaction posture, and network allowlist through a
  complete `model_provider_endpoint` grant. Generic network or credential
  approval does not satisfy this gate.
- Release binaries, containers, and tracked source must not include model weights
  or inference-runtime payloads unless a future distribution decision records
  license, size, security, cross-platform, update, and rollback evidence.
