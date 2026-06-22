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

The T1/T2/T3 command surface establishes the lifecycle skeleton, configuration
contract, and agent-native output contract:

- `relia init` creates `relia.yaml` if it is missing and is idempotent when the
  file already exists. It also creates the repo-native artifact skeleton under
  `.relia/` and `memory/`.
- `relia check` validates the repo-local operating pack baseline, required
  Phase 0 schemas, explicit privacy defaults, fail-closed redaction defaults,
  local model-artifact posture, and any `memory/rules/*.yaml` artifacts present.
- `relia ingest --input <path>` reads local JSON or JSONL outcome events,
  redacts and entropy-scans them before persistence, normalizes canonical
  experience records, and upserts monthly shards under `.relia/experiences/`.
  This task slice is offline only; live GitHub intake remains behind future
  credential and network gates.
- `--json` always emits the stable command result envelope.
- piped or non-interactive stdout defaults to JSON.
- `--quiet` and `--compact` emit compact JSON while preserving status,
  evidence refs, typed errors, and exit codes.

The command result schema is `schemas/command-result.schema.json`. Stable
exit-code examples for codes `0` through `9` are in
`examples/command-results/exit-code-examples.json`.

The config in `relia.yaml` is local-only by default: code, diffs, logs, and
experience records are not sent anywhere; share scope is `private`; redaction
uses entropy scanning and fails closed. `distill.embeddings: signature` is the
zero-install default. `distill.embeddings: local` requires a pulled model
manifest and fails closed with exit `8` when it is absent.

Ingested records must include PR provenance. Known token shapes and
secret-named fields are redacted before `.relia/experiences/*.jsonl` is written;
opaque high-entropy values that Relia cannot classify fail closed with exit `6`
and do not create or update a shard.

Phase 0 artifact schemas live under `schemas/` for experience records, outcome
evidence, failure signatures, memory rules, coverage maps, risk assessments,
recurrence reports, compiled context, command results, and redaction config.
Every schema is versioned and requires forward-compatible `metadata`.

Provider-backed distill work requires a complete `model_provider_endpoint`
grant naming provider, model, endpoint or `base_url`, credential environment,
budget posture, redaction posture, and allowlist. Local embedding artifact pulls
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
