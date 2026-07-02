package distill

import (
	"fmt"
	"testing"
)

func TestStatusCountsCountsRuleStatuses(t *testing.T) {
	rules := []Rule{{Status: "candidate"}, {Status: "active"}, {Status: "candidate"}}

	got := StatusCounts(rules)
	if got["candidate"] != 2 || got["active"] != 1 || got["stale"] != 0 {
		t.Fatalf("StatusCounts = %#v", got)
	}
}

func TestDraftedRuleDataBindsArtifactPathsByRuleID(t *testing.T) {
	rules := []Rule{
		{ID: "avoid-build", Kind: "avoid", Status: "candidate", ReviewLabel: "suggested", Confidence: 0.75, Metadata: RuleMetadata{ConfidenceLabel: "high"}},
	}

	got := DraftedRuleData(rules, []string{"memory/rules/avoid-build.yaml"})
	want := []map[string]any{{
		"id":               "avoid-build",
		"kind":             "avoid",
		"status":           "candidate",
		"review_label":     "suggested",
		"confidence":       0.75,
		"confidence_label": "high",
		"path":             "memory/rules/avoid-build.yaml",
	}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("DraftedRuleData = %#v, want %#v", got, want)
	}
}
