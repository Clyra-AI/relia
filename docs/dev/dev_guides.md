# Relia Developer Guide

## Toolchain Pins

| Tool | Version |
|---|---:|
| Go | 1.26.4 |

## Validation Matrix

- make lint-fast: repo operating pack and layout checks.
- make test-fast: Go unit tests.
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

## CI And PR Lifecycle

- GitHub Actions workflow: .github/workflows/validate.yml.
- Required local command: make prepush-full.
- Security scanner: opt-in CodeQL for Go via .github/workflows/codeql.yml.
  Enable GitHub Code Security/code scanning and set repository variable
  CODEQL_ENABLED=true before expecting CodeQL to run on PRs.
- PR lifecycle report path: .factory/artifacts/pr-lifecycle/<work_item_id>/pr-lifecycle-report.json.

## Bootstrap Rules

- Deterministic bootstrap must not require network, sandbox credentials, or model keys.
- Evidence artifacts must use repo-relative paths.
- New dependencies must be pinned and justified.
