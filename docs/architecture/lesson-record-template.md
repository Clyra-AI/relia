# Lesson Record Template

Status: Template
Scope: Reviewed Relia lesson or memory-grain candidate.

Lesson records are narrow, evidence-backed, and expiring. They are not broad
style rules, customer transcripts, or hidden product requirements.

## Metadata

- lesson_id:
- owner:
- status: draft | approved | expired | rejected
- created_at:
- expires_at_or_revisit_trigger:
- source_intake_ref:
- related_acceptance_items:
- related_task_refs:

## Lesson

- statement:
- applicability:
- non_applicability:
- affected_surfaces:
- expected_user_value:

## Evidence

- evidence_refs:
- fixture_refs:
- validation_commands:
- proof_level: syntax | source_evidence | workflow_behavior | user_visible_behavior
- confidence:

## Safety And Privacy

- contains_customer_data: no
- redaction_refs:
- approval_ref:
- known_risks:
- rollback_or_retirement_path:

## Promotion Decision

- decision: promote | hold | reject | expire
- reviewer:
- decision_date:
- rationale:
- follow_up_task_refs:

## Required Checks

- Lesson has at least one evidence ref.
- Lesson has explicit applicability and non-applicability.
- Lesson has an owner and expiry/revisit trigger.
- No raw customer data, credentials, owner handles, private endpoints, or
  machine-local paths are present.
- Product behavior changes are represented by task packets and validation
  evidence, not by this lesson record alone.
