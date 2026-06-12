# Factory Artifacts

- .factory/artifacts/: committed planning, validation, closure, and handoff artifacts.
- .factory/tmp/: ignored local scratch space.
- .factory/factoryd.example.json: safe repo-local daemon configuration template.
- .factory/factoryd.autoship.example.json: explicit full-loop daemon configuration template for protected GitHub repos.
- .factoryd/: ignored local daemon state, queue, worktrees, claims, events, and run reports.

The initial PRD-to-plan artifacts are under:

~~~text
.factory/artifacts/prd-to-plan/relia-mvp/
~~~

Post-PRD audit and review findings are ingested into separate mission folders:

~~~sh
FACTORY_REPO=/path/to/factory factoryd ingest --config .factory/factoryd.example.json --repo relia --kind audit --input product/audits/<mission>.md --mission <mission> --json
FACTORY_REPO=/path/to/factory factoryd ingest --config .factory/factoryd.example.json --repo relia --kind review --input product/reviews/<mission>.md --mission <mission> --json
~~~

Generated post-PRD artifacts live under `.factory/artifacts/post-prd/<mission>/`.

Live approval, credential, network, model-provider, and model-artifact grants
belong in the active `.factory/factoryd.json` daemon config. PRD-derived task
packets may carry planning-time seed grants that describe the required
capability, but operator approvals should not be recorded by mutating generated
task packets. Checked-in example and autoship template configs are not active
approval records and must not satisfy live/model approval gates.
