# Relia Workflow Contract

Version: 0.1
Status: Normative

## Work Signal

This repo accepts work from:

- [docs/product/prd.md](docs/product/prd.md)
- Factory task packets under .factory/artifacts/
- issues and PRs after repo activation

## Normal Factory Chain

1. scout-context
2. execution-compiler
3. task-executor
4. validation-gate
5. commit-push
6. post-merge-monitor

## Supervised Autoship

When the operator wants daemon-led implementation with bounded oversight, use
the Factory `autoship-supervisor` skill from the Relia repo root and bind it to
exactly one task ID, for example `T12`.

The supervisor may start or resume `factoryd run` with the autoship config, but
`factoryd` remains the implementation and shipping engine. The supervisor only
classifies blockers, records human acceptance when a task explicitly requires
approval evidence, applies narrow manual repair after daemon stop or
non-convergence, and confirms CI, Codex review, merge, post-merge, and
item-level scope closure. Any intervention must be recorded as an
`autoship_supervisor_report` under
`.factory/artifacts/supervisor-runs/<task_id>/`.

Do not use supervised autoship to widen task scope, bypass lifecycle gates,
edit PRD-derived control artifacts directly, or close acceptance with broad
labels instead of item-level evidence.

## Validation Lanes

- Fast lane: make lint-fast, make test-fast
- Contract lane: make test-contracts
- Full lane: make prepush-full
- Required PR checks: validate, CodeQL analyze

## Post-PRD Findings

When `app-audit` or `code-review` produces material follow-up work, save the
finding list in the repo and run:

```sh
FACTORY_REPO=/path/to/factory factoryd ingest --config .factory/factoryd.example.json --repo relia --kind audit --input product/audits/<mission>.md --mission <mission> --json
FACTORY_REPO=/path/to/factory factoryd ingest --config .factory/factoryd.example.json --repo relia --kind review --input product/reviews/<mission>.md --mission <mission> --json
```

The generated `.factory/artifacts/post-prd/<mission>/` artifacts are the
governed source for execution. Do not edit `docs/product/prd.md` unless a human
explicitly promotes a finding into product scope.

## Post-MVP Learning Intake

Customer-derived failures are intake candidates, not immediate product
requirements. Record them with
[docs/dev/customer-failure-intake-template.md](docs/dev/customer-failure-intake-template.md),
redact them before they enter committed artifacts, and promote only synthetic or
public-safe fixtures. Lessons intended for reuse must follow
[docs/architecture/lesson-record-template.md](docs/architecture/lesson-record-template.md)
and include owner, evidence, applicability, non-applicability, and expiry.

## Stop Conditions

Stop and request a human decision if runtime pins, distribution target, credential posture, network posture, or PRD scope boundaries need to change.

Scanner-gated changes cannot close without CodeQL status evidence or an
approved scanner exception.
Release or completion claims cannot close unless the final
release/demo/product-signals delivery slice is marked as the public release
boundary and all required MVP acceptance items have evidence.
