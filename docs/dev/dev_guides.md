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
`duration_ms`, `redaction_status`, and a forward-compatible `metadata` object
with the Relia version and schema id. Human-readable output is only the default
when stdout is an interactive terminal and no machine-readable flag is present.

T2 extends `relia check` beyond file presence. It validates the versioned
`relia.yaml` privacy defaults, the schema contract files under `schemas/`,
reviewed rules under `memory/rules/*.yaml`, and the artifact layout contract
surfaced in the command-result `data` object. The default config includes the
documented `advise` and `badge` sections, with `advise.enabled: false` so
bootstrap does not perform live PR advisory work by default.
Unsafe redaction settings fail closed with exit `6`; non-private MVP sharing
posture also fails closed with exit `6`; explicit `embeddings: local` without a pulled
artifact fails with exit `8`; disabled `distill.review_required` and unknown
provider/config values fail with exit `2`; invalid memory-rule provenance,
experience citations, or missing/non-accepted active review labels fail with
exit `4`. The executable
config schema accepts the documented PRD `advise` and `badge` sections without
enabling live network or credentialed work by default. Config arrays may use
block or inline YAML sequence syntax, including `redaction.patterns: [api_key,
token, password, secret]`. Version-only PRD bootstrap configs are normalized to
`schema_version: "1.0"` with MVP-safe privacy and artifact defaults before schema
validation. The risk-assessment schema keeps PRD-required matched rules with
citations and coverage stats explicit, and the recurrence-report headline keeps
flake-discounted and uncertain-attribution counts explicit.

## Structured Data, Proof, Budgets, And Redaction

- PR, check-run, experience, memory, MCP, assessment, report, and config data
  must be handled through parsers, schemas, or stable APIs.
- Phase 0 schema contracts must declare `schema_version`, required fields,
  allowed enum values, a forward-compatible `metadata` object, and
  `x-relia_error_mapping` for stable CLI exits.
- Experience record action blocks use canonical `pr` and `commits` fields;
  experience, coverage, and recurrence artifact schemas use canonical repo
  identifier strings, and recurrence report `error_recurrence_rate` is a
  bounded `0` through `1` proportion.
- Risk-assessment artifacts must preserve `matched_rules` with citations and
  `coverage_stats`, and recurrence reports must preserve
  `attribution_uncertain_count` and `flake_discounted_count`.
- Outcome and failure-signature schemas must use the PRD taxonomy names:
  `revert`, `merge_clean`, and `type_failure`. Memory-rule schemas must keep
  the durable artifact fields `id`, `status`, `evidence`, and PR-backed
  `provenance`.
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
