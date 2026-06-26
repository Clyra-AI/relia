# Relia Demo Memory Page

Backtest baseline: **21.4%** headline ERR from 3 confirmed recurrences across
14 agent-attributed failures. The seeded corpus also records 3 flake-discounted
failures and 3 uncertain attribution cases excluded from the headline: PRs
#233, #252, and #271.

## Candidate: avoid-mocking-datetime-directly

Do not mock the billing clock directly in rollover tests. Use the freeze-time
fixture that held in [PR #210](https://github.com/Clyra-AI/relia-demo-seed/pull/210).

Evidence:

- [PR #142](https://github.com/Clyra-AI/relia-demo-seed/pull/142)
- [PR #187](https://github.com/Clyra-AI/relia-demo-seed/pull/187)
- [PR #203](https://github.com/Clyra-AI/relia-demo-seed/pull/203)

## Candidate: avoid-blind-schema-snapshot-regeneration

Do not regenerate API schema snapshots without checking required error-shape
assertions.

Evidence:

- [PR #244](https://github.com/Clyra-AI/relia-demo-seed/pull/244)
- [PR #258](https://github.com/Clyra-AI/relia-demo-seed/pull/258)

## Lifecycle Outcomes

### Contradicted: avoid-blind-schema-snapshot-regeneration

No longer served. The rule is contradicted by clean schema snapshot updates that
kept the required error-shape assertions intact.

Evidence:

- Original failures: [PR #244](https://github.com/Clyra-AI/relia-demo-seed/pull/244), [PR #258](https://github.com/Clyra-AI/relia-demo-seed/pull/258)
- Contradictions: [PR #282](https://github.com/Clyra-AI/relia-demo-seed/pull/282), [PR #286](https://github.com/Clyra-AI/relia-demo-seed/pull/286)

### Stale: avoid-mocking-datetime-directly

No longer served. The scoped test path `tests/billing/test_invoice_time.py` was
removed in [PR #293](https://github.com/Clyra-AI/relia-demo-seed/pull/293).

Evidence:

- Original failures: [PR #142](https://github.com/Clyra-AI/relia-demo-seed/pull/142), [PR #187](https://github.com/Clyra-AI/relia-demo-seed/pull/187), [PR #203](https://github.com/Clyra-AI/relia-demo-seed/pull/203)
- Scope deletion: [PR #293](https://github.com/Clyra-AI/relia-demo-seed/pull/293)

## Discounted

[PR #214](https://github.com/Clyra-AI/relia-demo-seed/pull/214),
[PR #216](https://github.com/Clyra-AI/relia-demo-seed/pull/216), and
[PR #219](https://github.com/Clyra-AI/relia-demo-seed/pull/219) are marked as
flake-discounted repeated notification retry failures. The planted flakes do not
draft rules.

## Redaction Proof

Seeded secret fixtures appear only as `[REDACTED:token]` and
`[REDACTED:secret]` in demo artifacts.
