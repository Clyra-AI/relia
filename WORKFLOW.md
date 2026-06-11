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

## Stop Conditions

Stop and request a human decision if runtime pins, distribution target, credential posture, network posture, or PRD scope boundaries need to change.

Scanner-gated changes cannot close without CodeQL status evidence or an
approved scanner exception.
Release or completion claims cannot close unless the final
release/demo/product-signals delivery slice is marked as the public release
boundary and all required MVP acceptance items have evidence.
