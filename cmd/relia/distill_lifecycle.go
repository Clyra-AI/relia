package main

func preserveExistingAcceptedRuleLifecycle(rule distilledRule, document yamlDocument) distilledRule {
	if document.Scalars["status"].Value != "active" ||
		document.Scalars["review.label"].Value != "accepted" ||
		rule.Status != "candidate" {
		return rule
	}
	rule.Status = "active"
	rule.ReviewLabel = "accepted"
	rule.ReviewGate = document.Scalars["review.gate"].Value
	rule.ReviewDecision = document.Scalars["review.decision"].Value
	rule.ReviewedBy = document.Scalars["review.reviewed_by"].Value
	rule.DecisionRef = document.Scalars["review.decision_ref"].Value
	rule.MergedInto = document.Scalars["review.merged_into"].Value
	rule.Metadata.LifecycleReason = "previous accepted review preserved"
	return rule
}
