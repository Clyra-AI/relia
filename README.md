# Relia

Relia is outcome memory for coding agents. This repository is the public OSS MVP
workspace, governed by `docs/product/prd.md` and Factory artifacts under
`.factory/artifacts/`.

## Start

~~~sh
make prepush-full
GOCACHE="${TMPDIR:-/tmp}/relia-go-build" go run ./cmd/relia --json check
FACTORY_REPO=/path/to/factory factoryd doctor --config .factory/factoryd.example.json --repo relia --json
FACTORY_REPO=/path/to/factory factoryd run --config .factory/factoryd.example.json --repo relia --dry-run --json
# After branch protection, CI, and review gates are proven:
FACTORY_REPO=/path/to/factory factoryd run --config .factory/factoryd.autoship.example.json --repo relia --loop --max-tasks 1 --json
~~~

## CLI Baseline

The T1 command surface establishes the lifecycle skeleton and agent-native
output contract:

- `relia init` creates `relia.yaml` if it is missing, preserves an existing
  config, and creates the repo-native artifact directory skeleton.
- `relia check` validates the repo-local operating pack baseline, schema
  contract files, fail-closed privacy defaults, and reviewed memory rules under
  `memory/rules/*.yaml`.
- `--json` always emits the stable command result envelope.
- piped or non-interactive stdout defaults to JSON.
- `--quiet` and `--compact` emit compact JSON while preserving status,
  evidence refs, typed errors, and exit codes.

The command result schema is `schemas/command-result.schema.json`. Stable
exit-code examples for codes `0` through `9` are in
`examples/command-results/exit-code-examples.json`. Command results include a
`metadata` object with the Relia version and schema id.

The default `relia.yaml` is versioned with `schema_version: "1.0"`, keeps
experience shards local by default (`commit_experiences: false`), keeps future
organization sharing disabled (`share_scope: private`, `org_eligible: false`),
uses deterministic `embeddings: signature`, and requires fail-closed redaction
with entropy scanning. It includes the documented `advise` and `badge` sections
for the PR advisory loop with `advise.enabled: false` by default, while the MVP
contract keeps `distill.review_required: true` mandatory. Existing PRD-style
configs that declare only `version: 1` are normalized to the MVP-safe
`schema_version: "1.0"` defaults during `relia check`.

Phase 0 artifact schemas live under `schemas/` for experience records, outcome
evidence, failure signatures, memory rules, coverage maps, risk assessments,
recurrence reports, compiled context, command results, redaction config, and the
repo config contract. Outcome schemas use the PRD names `ci_failure`, `revert`,
`review_correction`, `merge_clean`, and `fix_held`; type-check signatures use
`type_failure`. Experience records use canonical action fields `pr` and
`commits`, canonical outcome field `terminal`, and embedded signatures retain
signature class, check name, key, message fingerprint, and extraction confidence
so recurrence pairing can be computed from the shard. Experience, coverage, and
recurrence artifacts use
canonical repo identifier strings such as `owner/name`, and recurrence report
ERR values are bounded proportions from `0` through `1`. Recurrence headlines
preserve `attribution_uncertain_count` and `flake_discounted_count`, and risk
assessments require `matched_rules` with citations plus `coverage_stats` for the
OOD signal. Memory rules use the documented durable-artifact shape with
`object_type`, `schema_version`, `id`, `kind`, `status`, `statement`,
`confidence`, `evidence`, `review`, `scope`, PR-backed `provenance`, and
`metadata`. Active rules must include experience citations, provenance entries,
existing scoped paths, and an accepted review label before `relia check` reports
success. Playbook rules must cite `merge_clean` or `fix_held` provenance.

Provider-backed distill work requires a complete `model_provider_endpoint`
grant in config before `embeddings: provider` is accepted: `provider`, `model`,
`endpoint` or `base_url`, `credential_env`, `budget_posture`,
`redaction_posture`, and non-empty `allowlist`. Local embedding artifact pulls
require a separate `model_artifact_pull` grant. Generic network or credential
approval does not satisfy either model-specific gate.

## Post-PRD audit or review findings

Save material findings from `app-audit` or `code-review` as repo-local markdown
under `product/audits/` or `product/reviews/`, then ingest them:

~~~sh
FACTORY_REPO=/path/to/factory factoryd ingest --config .factory/factoryd.example.json --repo relia --kind audit --input product/audits/<mission>.md --mission <mission> --json
FACTORY_REPO=/path/to/factory factoryd ingest --config .factory/factoryd.example.json --repo relia --kind review --input product/reviews/<mission>.md --mission <mission> --json
~~~

## Customer-derived failures

Customer-derived failures are not committed directly. Use
`docs/dev/customer-failure-intake-template.md` for redacted intake and
`docs/architecture/lesson-record-template.md` for reviewed lesson candidates.
Only synthetic, public-safe, owner-approved fixtures may become reusable tests
or product lessons.
