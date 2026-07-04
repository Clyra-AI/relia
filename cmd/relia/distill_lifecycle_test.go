package main

import "testing"

func TestPreserveExistingAcceptedRuleLifecycleDefaultsLegacyDecisionApproved(t *testing.T) {
	document := parseRuleDocForTest(t, `object_type: relia.memory_rule
schema_version: "1.0"
id: legacy-accepted-rule
kind: avoid
status: active
statement: Avoid the legacy pattern.
review:
  label: accepted
  statement_origin: cluster_summary
metadata:
  lifecycle_reason: existing approval predates decision fields
`)
	rule := distilledRule{
		ID:             "legacy-accepted-rule",
		Kind:           "avoid",
		Status:         "candidate",
		ReviewLabel:    "suggested",
		ReviewDecision: "",
	}

	preserved := preserveExistingAcceptedRuleLifecycle(rule, document)

	if preserved.Status != "active" ||
		preserved.ReviewLabel != "accepted" ||
		preserved.ReviewDecision != "approved" {
		t.Fatalf("preserved lifecycle = status %q label %q decision %q", preserved.Status, preserved.ReviewLabel, preserved.ReviewDecision)
	}
	if preserved.Metadata.LifecycleReason != "existing approval predates decision fields" {
		t.Fatalf("lifecycle reason = %q", preserved.Metadata.LifecycleReason)
	}
}

func TestPreserveExistingAcceptedRuleLifecycleKeepsRetiredDecisionForNonCandidate(t *testing.T) {
	document := parseRuleDocForTest(t, `object_type: relia.memory_rule
schema_version: "1.0"
id: merged-rule
kind: playbook
status: retired
statement: Prefer the canonical rule.
review:
  label: merged
  gate: human_review
  decision: merged
  reviewed_by: maintainer
  decision_ref: relia review merge --rule merged-rule --into canonical-rule
  merged_into: canonical-rule
metadata:
  lifecycle_reason: duplicate of canonical rule
`)
	rule := distilledRule{
		ID:             "merged-rule",
		Kind:           "playbook",
		Status:         "contradicted",
		ReviewLabel:    "needs_user_input",
		ReviewDecision: "pending",
	}

	preserved := preserveExistingAcceptedRuleLifecycle(rule, document)

	if preserved.Status != "retired" ||
		preserved.ReviewDecision != "merged" ||
		preserved.MergedInto != "canonical-rule" ||
		preserved.ReviewedBy != "maintainer" {
		t.Fatalf("preserved retired lifecycle = %#v", preserved)
	}
	if preserved.Metadata.LifecycleReason != "duplicate of canonical rule" {
		t.Fatalf("lifecycle reason = %q", preserved.Metadata.LifecycleReason)
	}
}
