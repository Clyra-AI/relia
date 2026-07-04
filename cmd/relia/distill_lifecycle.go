package main

func preserveExistingAcceptedRuleLifecycle(rule distilledRule, document yamlDocument) distilledRule {
	if rule.Status != "candidate" {
		return rule
	}
	status := document.Scalars["status"].Value
	reviewLabel := document.Scalars["review.label"].Value
	reviewDecision := document.Scalars["review.decision"].Value
	lifecycleReason := ""
	switch {
	case status == "active" && reviewLabel == "accepted":
		lifecycleReason = "previous accepted review preserved"
	case status == "retired" && (reviewDecision == "merged" || reviewDecision == "rejected"):
		lifecycleReason = "previous retired review preserved"
	default:
		return rule
	}
	rule.Status = status
	rule.ReviewLabel = reviewLabel
	rule.ReviewGate = document.Scalars["review.gate"].Value
	rule.ReviewDecision = reviewDecision
	rule.ReviewedBy = document.Scalars["review.reviewed_by"].Value
	rule.DecisionRef = document.Scalars["review.decision_ref"].Value
	rule.MergedInto = document.Scalars["review.merged_into"].Value
	rule.Metadata.LifecycleReason = lifecycleReason
	return rule
}
