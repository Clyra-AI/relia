# Customer Failure Intake Template

Status: Template
Scope: Post-MVP learning loop only.

Use this template when a customer repo, support case, audit, or review finding
suggests a reusable Relia lesson. Records created from this template must stay
under the approved repo-local intake path and must not contain raw customer
data.

## Intake Metadata

- intake_id:
- source_type: customer_repo | support_case | audit | review | other
- source_ref:
- owner:
- received_at:
- related_task_or_acceptance_item:
- proposed_fixture_path:

## Failure Summary

- observed_failure_mode:
- expected_behavior:
- actual_behavior:
- affected_relia_surface:
- customer_identifiers_removed: yes | no
- safe_to_reproduce_publicly: yes | no

## Redaction And Privacy

- redaction_status: complete | incomplete | not_applicable
- redacted_fields:
- nested_fields_checked:
- machine_local_paths_removed: yes | no
- credentials_or_tokens_removed: yes | no
- private_endpoints_removed: yes | no
- owner_handles_removed_or_pseudonymized: yes | no
- reviewer:
- review_decision: promote | hold | reject
- review_notes:

## Fixture Promotion

- synthetic_fixture_available: yes | no
- fixture_generation_method:
- fixture_expected_outcome:
- negative_case_required: yes | no
- validation_command:
- proof_refs:
- reason_not_promoted:

## Lesson Grain

- lesson_statement:
- applicability:
- non_applicability:
- expiry_or_revisit_trigger:
- evidence_refs:
- owner:
- promotion_decision:

## Required Checks Before Promotion

- No raw customer data is committed.
- The fixture is synthetic, public, or explicitly approved for repository use.
- The lesson has applicability limits and an expiry/revisit trigger.
- The resulting task packet cites evidence refs and acceptance item refs.
- The generated behavior remains advisory unless the PRD explicitly requires
  enforcement.
