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

## Stop Conditions

Stop and request a human decision if runtime pins, distribution target, credential posture, network posture, or PRD scope boundaries need to change.
