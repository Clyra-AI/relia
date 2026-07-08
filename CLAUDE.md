<!-- relia:begin (generated; edit rules in memory/rules/, not here) -->
Relia managed context (schema_version=1.0; relia_version=0.0.0-dev; source=memory/rules).
Do not edit this block directly; edit reviewed rules in `memory/rules/` and run `relia compile`.
Non-MCP agents should treat only the active accepted rules below as Relia memory.

- `avoid-direct-datetime-in-assessment-fixture` (avoid, confidence 0.86, 3 citations): Use the billing time-control fixture instead of reading datetime.utcnow directly in rollover logic.
  receipts: [PR #142](https://github.com/Clyra-AI/relia-demo-seed/pull/142) - outcome `ci_failure`; [PR #187](https://github.com/Clyra-AI/relia-demo-seed/pull/187) - outcome `ci_failure`; [PR #203](https://github.com/Clyra-AI/relia-demo-seed/pull/203) - outcome `ci_failure`
  source: `memory/rules/demo-assessment-active-rule.yaml`
<!-- relia:end -->
