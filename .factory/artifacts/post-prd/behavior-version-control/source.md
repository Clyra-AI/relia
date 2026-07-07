# Behavior Version Control

Post-PRD product idea for Relia: when agent behavior gets worse after a rule,
memory, or instruction change, Relia should identify the exact behavior change
candidate with receipts and recommend a rollback path. This is post-MVP product
scope unless a later PRD revision promotes it into MVP scope.

1. Behavior change provenance inventory

What: Record versioned changes to memory rules, compiled context, AGENTS/CLAUDE blocks, and Relia-owned instruction artifacts as local behavior-change candidates with commit refs, diff refs, owner/reviewer metadata when available, source rule IDs, lifecycle status, and affected path/signal scope.

How: Build a deterministic offline inventory from local git history, committed rule files, Relia lifecycle metadata, and existing report artifacts. The inventory must not mutate rules, compile targets, or reviewed memory by itself.

Where:
- schemas/
- examples/
- docs/product/
- docs/dev/
- docs/architecture/
- cmd/relia/
- internal/memory/
- internal/backtest/
- internal/assess/

Dependency: Run after T18 because lesson grain, owner, evidence, applicability, and expiry rules must exist before behavior-change attribution can be trusted.

2. ERR regression attribution

What: Compare repeat-failure and ERR deltas before and after behavior-change candidates, then report candidate links with confidence, supporting receipts, diff refs, comparison windows, sample sizes, and excluded confounders.

How: Reuse existing recurrence signatures, baselines, flake discounts, attribution confidence, contradiction handling, and stale-rule logic. When evidence is thin, classify the result as attribution_uncertain instead of blaming a behavior change.

Where:
- schemas/
- examples/
- docs/product/
- docs/dev/
- docs/architecture/
- cmd/relia/
- internal/backtest/
- internal/memory/
- internal/assess/

Dependency: Run after behavior change provenance inventory.

3. Rollback recommendation report

What: Surface a product-safe advisory such as "repeat-failure rate rose after rule X v3 was approved; here is the rule/instruction diff, confidence, evidence, and recommended rollback path." The recommendation is advisory only and never auto-applies a rollback.

How: Add structured CLI/report fields and fixtures that show the diff ref, candidate rollback action, expected effect, confidence, and required human review. Tests must prove redaction is preserved and no rule, memory page, compiled block, or instruction file changes unless a later explicit task makes that mutation.

Where:
- schemas/
- examples/
- docs/product/
- docs/dev/
- docs/architecture/
- cmd/relia/
- internal/backtest/
- internal/memory/
- internal/assess/
- internal/advise/

Dependency: Run after ERR regression attribution.
