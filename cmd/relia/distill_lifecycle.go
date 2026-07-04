package main

func preserveExistingAcceptedRuleLifecycle(rule distilledRule, document yamlDocument) distilledRule {
	status := document.Scalars["status"].Value
	reviewLabel := document.Scalars["review.label"].Value
	reviewDecision := document.Scalars["review.decision"].Value
	lifecycleReason := ""
	switch {
	case rule.Status == "candidate" && status == "active" && reviewLabel == "accepted":
		lifecycleReason = "previous accepted review preserved"
	case status == "retired" && (reviewDecision == "merged" || reviewDecision == "rejected"):
		lifecycleReason = "previous retired review preserved"
	default:
		return rule
	}
	if existingReason := document.Scalars["metadata.lifecycle_reason"].Value; existingReason != "" {
		lifecycleReason = existingReason
	}
	rule.Status = status
	rule.ReviewLabel = reviewLabel
	rule.ReviewGate = document.Scalars["review.gate"].Value
	rule.ReviewDecision = reviewDecision
	if status == "active" && reviewLabel == "accepted" && rule.ReviewDecision == "" {
		rule.ReviewDecision = "approved"
	}
	rule.ReviewedBy = document.Scalars["review.reviewed_by"].Value
	rule.DecisionRef = document.Scalars["review.decision_ref"].Value
	rule.MergedInto = document.Scalars["review.merged_into"].Value
	rule.Metadata.LifecycleReason = lifecycleReason
	return rule
}
