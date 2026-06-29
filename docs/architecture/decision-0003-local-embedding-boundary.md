# Decision 0003: Local Embedding Boundary

Status: accepted
Date: 2026-06-29

## Context

T7 implements the PRD local model artifact gate for distillation while the
Factory task runner has no network grant, no live credentials, and no
`model_artifact_pull` approval. The PRD requires `embeddings: signature` to
remain the zero-install deterministic fallback and requires `embeddings: local`
to fail closed when the local artifact is missing, stale, or
digest-mismatched.

Local embedding inference would require a separate runtime choice with license,
size, security, update, rollback, cross-platform, and cache-location evidence.
No such runtime decision is approved in this task.

## Decision

T7 does not introduce a local embedding inference runtime. The implemented
boundary is manifest recording and fail-closed validation only:

- `relia models pull` records metadata for an already-present local artifact.
- The manifest must include model ID, version, source URL, license, SHA-256
  digest, cache path, update policy, rollback policy, and ready/stale status.
- The artifact must live under `.relia/models/` and match the recorded digest.
- `embeddings: local` fails closed with exit `8` when the manifest or artifact
  is missing, stale, escaped from `.relia/models/`, or digest-mismatched.
- `embeddings: signature` remains deterministic, no-network, no-model, and
  records signature-only cluster provenance.

Future work that actually computes embedding vectors must add a new ADR before
choosing a pure Go library, ONNX/runtime binding, or external local process.

## Consequences

The MVP can prove the model artifact safety boundary without bundling model
weights, adding inference-runtime payloads, or attempting downloads in source,
binaries, containers, or release artifacts. Local semantic refinement remains a
future explicitly approved implementation step; until then, deterministic
signature clustering is the trustworthy fallback.
