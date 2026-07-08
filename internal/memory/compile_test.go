package memory

import "testing"

func TestSelectCompiledRulesRequiresExplicitApprovalFields(t *testing.T) {
	rules := []RuleSummary{
		{
			ID:             "approved",
			Status:         "active",
			ReviewLabel:    "accepted",
			ReviewGate:     "human_review",
			ReviewDecision: "approved",
			Confidence:     "0.8",
		},
		{
			ID:          "missing-review-fields",
			Status:      "active",
			ReviewLabel: "accepted",
			Confidence:  "0.9",
		},
		{
			ID:             "pending",
			Status:         "active",
			ReviewLabel:    "accepted",
			ReviewGate:     "human_review",
			ReviewDecision: "pending",
			Confidence:     "0.95",
		},
	}

	selected := SelectCompiledRules(rules, 25)

	if len(selected) != 1 || selected[0].ID != "approved" {
		t.Fatalf("selected = %#v", selected)
	}
}
