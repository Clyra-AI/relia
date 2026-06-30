# Decision 0004: Provider And Advisory Boundary

Status: Accepted
Date: 2026-06-30

## Context

T9 introduces provider-backed distillation adapters, a local serve surface, and
the GitHub advisory loop. These surfaces touch model-provider credentials,
network calls, GitHub API writes, command-result contracts, and PR comments.
The repository contract keeps Relia offline by default and requires a complete
`model_provider_endpoint` grant before provider embeddings or LLM rule drafting
can call a live endpoint.

## Decision

Relia will implement provider integrations as deterministic adapter and cost
plans until an approved grant exists. `relia distill` validates provider,
model, HTTPS `base_url`, credential environment variable name, cost cap, and
configured token unit costs; it estimates tokens and cost from redacted
experience records; and it fails closed before reading credential values or
opening a network connection without `model_provider_endpoint` approval.

`relia serve --format json` exposes the local MCP capability manifest for
`recall`, `assess`, and `coverage` over active accepted memory rules only.
Hosted or network serve transports fail closed in the MVP default posture.

`relia advise --input <diff>` reuses the assessment engine to write local
advisory state and comment artifacts. The GitHub Action reads those artifacts
and uses the explicit GitHub Actions token only to create or update one
hidden-marker advisory comment per PR.

## Consequences

- No hosted Relia account, provider credential, model key, or live network call
  is required for the default local workflow.
- Provider cost visibility is deterministic and testable without relying on
  external pricing tables or live billing APIs.
- LLM-drafted statements remain unavailable until the model-provider grant is
  complete; deterministic cluster-summary drafting remains the fallback.
- The PR advisory loop is opt-in through the workflow and is advisory-only by
  default.

## Rollback

Remove the T9 provider-plan, `serve`, and `advise` command paths; remove
`.github/workflows/relia-advisory.yml`; and delete generated
`.relia/reports/advisory-*` artifacts. Existing experience records, memory
rules, MEMORY.md, reports, and baselines remain portable.
