# Relia

Relia is outcome memory for coding agents, built from
[docs/product/prd.md](docs/product/prd.md). This repository is in the MVP
foundation stage: lifecycle controls, validation lanes, and the CLI command
result contract are established before product commands are filled in.

## CLI foundation

The primary binary entrypoint is `cmd/relia`.

~~~sh
go run ./cmd/relia --json
go run ./cmd/relia --compact status
go run ./cmd/relia --quiet version
~~~

Current implemented commands:

- `status`: emits the command-result envelope and contract artifact refs.
- `version`: emits the current development version.
- `help`: emits usage.

MVP commands such as `backtest`, `check`, `ingest`, `distill`, `review`,
`memory`, `compile`, `serve`, `assess`, `models pull`, `demo`, and `share` are
recognized as typed command stubs in this foundation slice. They return a
machine-readable `not_implemented` error with exit code `1` until their product
behavior lands in later task packets.

When stdout is not a TTY, Relia emits JSON automatically. `--json`, `--quiet`,
and `--compact` all preserve the stable `relia.command_result` envelope,
including status, evidence refs, typed errors, and exit codes. `--quiet` and
`--compact` use compact JSON. The schema lives at
`schemas/command-result.schema.json`; stable exit-code examples live under
`examples/command-results/`.

## Start

	~~~sh
	make prepush-full
	FACTORY_REPO=/path/to/factory factoryd doctor --config .factory/factoryd.example.json --repo relia --json
	FACTORY_REPO=/path/to/factory factoryd run --config .factory/factoryd.example.json --repo relia --dry-run --json
	# After branch protection, CI, and review gates are proven:
	FACTORY_REPO=/path/to/factory factoryd run --config .factory/factoryd.autoship.example.json --repo relia --loop --max-tasks 1 --json
	~~~

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
