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

The T1/T2/T3/T4.3/T5/T6/T7/T8/T9 command surface establishes the lifecycle
skeleton, configuration contract, assessment fixture slice, recurrence
backtest, deterministic distillation, reporting evidence, provider/advisory
approval boundaries, and agent-native output contract:

- `relia init` creates `relia.yaml` if it is missing and is idempotent when the
  file already exists. It also creates the repo-native artifact skeleton under
  `.relia/` and `memory/`.
- `relia check` validates the repo-local operating pack baseline, required
  Phase 0 schemas, explicit privacy defaults, fail-closed redaction defaults,
  local model-artifact posture, and any `memory/rules/*.yaml` artifacts present.
  Memory rule provenance must include GitHub PR URLs whose pull numbers match
  the recorded `pr` receipts.
- `relia ingest --input <path>` reads local JSON or JSONL outcome events,
  redacts and entropy-scans them before persistence, normalizes canonical
  experience records, and upserts monthly shards under `.relia/experiences/`.
  This task slice is offline only; live GitHub intake remains behind future
  credential and network gates. Events marked as agent self-reports or
  reflections fail before persistence.
- `relia backtest --window 180d` reads local `.relia/experiences/*.jsonl`
  shards, extracts deterministic signatures, pairs confirmed recurrences
  separately from possible recurrences, discounts flakes, and writes JSON plus
  HTML reports under `.relia/reports/`. Possible recurrences and discounted
  flakes are excluded from the headline ERR. A saved ERR baseline under
  `.relia/baselines/error-recurrence-baseline.json` is compared when present
  and labeled stale when the source artifact digest differs. The report and
  command result expose operator-ready metrics, including agent-attributed
  experience counts distinct from unique agent-attributed PR counts, top
  repeated mistakes, diagnostics, feedback text, and badge-ready fields
  generated from the JSON command result. Badge fields render as stale when
  artifact metadata reports `last_ingest_at` older than 30 days or more than
  20 merged PRs since ingest; missing ingest or activity metadata is also
  treated as stale before publishing a README badge. The recurrence gate is
  available through `relia.yaml` but remains off by default.
- `relia distill --format json` reads local redacted experience shards, or
  `relia distill --input <json-or-jsonl> --format json` reads one local
  redacted outcome file without persisting shards. It clusters canonical
  deterministic signature keys before signature ID fallback and writes
  candidate `avoid` and `playbook` rules under `memory/rules/`. Confidence is
  calculated from evidence count, the PRD default 90-day recency half-life,
  contradictions, flake discounts, and extraction confidence; confidence is
  capped at `0.6` until a draft has three confirmed experiences, and drafting
  models do not affect confidence. Drafted `cluster_summary` and future
  `llm_drafted` rules must carry their confidence-input and decay metadata. With
  the default `distill.review_required: true`, drafted rules are not active
  until reviewed; setting it to `false` is surfaced but does not auto-accept
  drafts in the MVP. Generated rules disclose
  `metadata.memory_source: verified_outcome_events`; canonical inputs marked
  as agent self-reports or reflections are rejected before rule files are
  written.
  Deleted scoped paths produce `stale` rules, and contradictory clean or held
  evidence produces `contradicted` rules.
  Provider-backed mode supports OpenAI-compatible and Anthropic adapter plans,
  but the local runner computes token and cost estimates, enforces
  `distill.max_cost_usd_per_run`, and fails closed before any provider call
  unless a complete `model_provider_endpoint` grant exists.
- `relia models pull --model-id ... --version ... --source-url ... --license
  ... --digest ... --cache-path ... --update-policy ... --rollback-policy ...`
  records the local embedding artifact manifest for an already-present local
  artifact. It performs no network download in the offline MVP runner path and
  validates the artifact digest before `embeddings: local` can pass.
- `relia review approve --rule <id>` moves a candidate memory rule to
  `active`. `relia review edit --rule <id> --statement <text>` marks the
  statement `human_authored` and keeps it in candidate review. `relia review
  reject --rule <id> --reason <text>` moves it to `retired`. The legacy
  `relia review --rule <id> --label accepted|suggested|needs_user_input` form
  remains supported.
- `relia memory --format json` renders `memory/MEMORY.md` with each rule's
  statement, confidence, lifecycle status, review label, and clickable PR
  provenance, separating active accepted rules as strong memory from candidate,
  stale, contradicted, and retired weak memory.
- `relia serve --format json` exposes the local MCP capability manifest for
  `recall`, `assess`, and `coverage` over active accepted rules only. Hosted or
  network transports fail closed unless explicitly approved.
- `relia assess --input <diff>` reads a local unified diff, compares touched
  paths with active local `memory/rules/*.yaml` rules, and emits a
  `relia.risk_assessment` payload with `match_high`, `match_medium`, or
  `no_coverage`.
- `relia advise --input <diff>` runs the same assessment engine and writes one
  advisory comment plan under `.relia/reports/`, including a stable hidden
  marker and diff fingerprint so the GitHub Action can edit one living comment
  or skip unchanged pushes.
- `--json` always emits the stable command result envelope.
- piped or non-interactive stdout defaults to JSON.
- interactive `relia backtest` output prints the operator summary, top repeated
  mistakes, report path, diagnostics, and badge text; the same data remains in
  the JSON envelope for automation.
- `--quiet` and `--compact` emit compact JSON while preserving status,
  evidence refs, typed errors, and exit codes.

The command result schema is `schemas/command-result.schema.json`. Stable
exit-code examples for codes `0` through `9` are in
`examples/command-results/exit-code-examples.json`.

## Demo Fixtures

The offline demo fixture corpus lives under `examples/demo/` and the bundled
static reports live under `examples/reports/`:

- `examples/demo/seeded-repo/prs.json` is the synthetic PR index used to resolve
  every demo citation without network access.
- `examples/demo/seeded-repo/outcomes.jsonl` is the seeded outcome stream used
  to reproduce the backtest headline.
- `examples/demo/attribution-precision-sample.json`,
  `examples/demo/flake-discount-fixtures.json`, and
  `examples/demo/redaction-fixtures/expected-redacted-artifacts.json` cover the
  attribution, flake-discount, and redaction honesty checks.
- `examples/demo/distill-review-lifecycle-fixtures.json` covers the planted
  recurrence draft, contradiction, and stale-path lifecycle outcomes with
  repo-relative evidence refs.
- `examples/demo/assessment-fixtures/assessment-fixtures.json` covers
  `relia assess` behavior for a planted-pattern diff that returns
  `match_high` with seeded PR citations and an unknown-path diff that returns
  `no_coverage`.
- `examples/reports/backtest-demo.json`,
  `examples/reports/backtest-demo.html`, and
  `examples/reports/memory-page-demo.md` are customer-safe static baselines.

`make test-contracts` verifies that the report numbers and PR citations are
reproducible from the seeded corpus, that the planted flake drafts no rule, that
uncertain attribution is excluded from precision, that distill/review lifecycle
fixtures keep contradicted and stale rules out of serving, that assessment
fixtures return deterministic repo-relative `match_high` and `no_coverage`
results, and that demo artifacts do not store seeded secrets.

The live backtest command is deterministic for the same window and local
experience artifacts: repeated runs use the latest experience timestamp as the
window anchor and produce the same report ID and artifact paths.

The live distill command is also deterministic for the same local experience
shards or explicit `--input` file and config. The default path is signature-only
and offline; local embedding mode fails closed unless a recorded manifest and
matching local artifact are present. Provider drafting and provider embeddings
produce an adapter/cost plan and fail closed unless an approved
`model_provider_endpoint` gate is present.

The config in `relia.yaml` is local-only by default: code, diffs, logs, and
experience records are not sent anywhere; share scope is `private`; redaction
uses entropy scanning and fails closed. `distill.embeddings: signature` is the
zero-install default. `distill.embeddings: local` requires a pulled model
manifest and fails closed with exit `8` when it is absent, stale, or
digest-mismatched.

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

The advisory workflow is `.github/workflows/relia-advisory.yml`. It runs on pull
requests, builds a local diff assessment, and uses the explicit GitHub Actions
token only in GitHub API steps, never while executing checked-out Relia code, to
create or update one Relia advisory comment. The checkout disables persisted
GitHub credentials before checked-out Relia code runs, and the workflow seeds
the local advisory state from all existing hidden-marker comment pages so reruns
can skip unchanged diff fingerprints. The publish step parses planner JSON
directly and invokes `gh` without sourcing planner-produced shell environment
text while the Actions token is present. It is advisory-only and exits successfully
when the local planner skips comments for new `covered_clean`, low-confidence,
unchanged-diff, disabled-advise, or `max_comments_per_pr: 0` cases; when an
existing marker is present and a later diff is `covered_clean`, it updates that
marker in place with a cleared advisory instead of leaving stale warning text.

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
