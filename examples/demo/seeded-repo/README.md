# Relia Seeded Demo Repo Fixture

This directory is a synthetic, customer-safe PR and outcome corpus for the
offline Relia demo. It is not derived from customer data.

- `prs.json` is the local PR index used to resolve demo citations.
- `outcomes.jsonl` is the seeded outcome stream used to reproduce the bundled
  backtest numbers and lifecycle fixture evidence.
- `../distill-review-lifecycle-fixtures.json` uses later clean-merge outcomes
  to prove contradicted and stale rules are kept out of serving without changing
  the seeded ERR denominator.

The bundled reports under `examples/reports/` must be reproducible from these
files without network access, credentials, model artifacts, or live GitHub API
calls.
