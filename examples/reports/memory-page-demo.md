# Relia Demo Memory Page

Backtest baseline: **21.4%** headline ERR from 3 confirmed recurrences across
14 agent-attributed failures. The seeded corpus also records 3 flake-discounted
failures and 3 uncertain attribution cases excluded from the headline: PRs
#233, #252, and #271.

## Serving Snapshot

No rules are served by this lifecycle fixture. The seeded recurrence draft and
the later contradiction and stale-path transitions are preserved below with
citations instead of being rendered as actionable recommendations.

## Lifecycle Outcomes

### Contradicted: avoid-blind-schema-snapshot-regeneration

No longer served. The rule is contradicted by clean schema snapshot updates that
kept the required error-shape assertions intact.

Evidence:

- Original failures: [PR #244](https://github.com/Clyra-AI/relia-demo-seed/pull/244), [PR #258](https://github.com/Clyra-AI/relia-demo-seed/pull/258)
- Contradictions: [PR #282](https://github.com/Clyra-AI/relia-demo-seed/pull/282), [PR #286](https://github.com/Clyra-AI/relia-demo-seed/pull/286)

### Stale: avoid-mocking-datetime-directly

No longer served. The scoped billing invoice path `packages/billing/invoice.py`
and test path `tests/billing/test_invoice_time.py` were removed in
[PR #293](https://github.com/Clyra-AI/relia-demo-seed/pull/293).

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
