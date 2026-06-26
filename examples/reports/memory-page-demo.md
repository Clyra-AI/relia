# Relia Demo Memory Page

Backtest baseline: **27.3%** headline ERR from 3 confirmed recurrences across
11 agent-attributed failures. The seeded corpus also records 1 flake-discounted
failure and 3 uncertain attribution cases excluded from the headline.

## Active Candidate: avoid-mocking-datetime-directly

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

## Discounted

[PR #219](https://github.com/Clyra-AI/relia-demo-seed/pull/219) is marked as
flake-discounted and does not draft a rule.
