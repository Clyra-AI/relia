# Relia Memory Artifacts

This directory is the default local artifact root for reviewed Relia memory.

The MVP contracts reserve:

- `memory/experiences.jsonl` for `relia.experience_record` objects using
  `schemas/experience-record.schema.json`.
- `memory/rules.jsonl` for `relia.memory_rule` objects using
  `schemas/memory-rule.schema.json`.
- `memory/MEMORY.md` for the rendered memory page with receipts.

Generated memory artifacts are local by default, are not customer-safe unless a
separate export process proves recursive redaction, and default to
`org_eligible: false`.
