# Relia Developer Guide

## Toolchain Pins

| Tool | Version |
|---|---:|
| Go | 1.26.4 |

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

## Structured Data, Proof, Budgets, And Redaction

- PR, check-run, experience, memory, MCP, assessment, report, and config data
  must be handled through parsers, schemas, or stable APIs.
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
