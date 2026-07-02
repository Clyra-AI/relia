package distill

import (
	"fmt"
	"testing"
	"time"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

func TestReviewLabelUsesLifecycleAndConfidence(t *testing.T) {
	cases := []struct {
		status     string
		confidence float64
		want       string
	}{
		{status: "active", confidence: 0.1, want: "accepted"},
		{status: "contradicted", confidence: 0.9, want: "needs_user_input"},
		{status: "candidate", confidence: 0.54, want: "needs_user_input"},
		{status: "candidate", confidence: 0.55, want: "suggested"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s-%.2f", tc.status, tc.confidence), func(t *testing.T) {
			if got := ReviewLabel(tc.status, tc.confidence); got != tc.want {
				t.Fatalf("ReviewLabel(%q, %.2f) = %q, want %q", tc.status, tc.confidence, got, tc.want)
			}
		})
	}
}

func TestRuleExperienceIDsTrimsAndDeduplicates(t *testing.T) {
	records := []backtestdoc.Experience{
		ruleProvenanceExperienceForTest(" exp-2 ", 2, "ci_failure"),
		ruleProvenanceExperienceForTest("exp-1", 1, "ci_failure"),
		ruleProvenanceExperienceForTest("exp-2", 2, "ci_failure"),
		ruleProvenanceExperienceForTest("", 3, "ci_failure"),
	}

	got := RuleExperienceIDs(records)
	if fmt.Sprint(got) != fmt.Sprint([]string{"exp-2", "exp-1"}) {
		t.Fatalf("RuleExperienceIDs = %#v", got)
	}
}

func TestRuleProvenanceRefsDeduplicatesAndSortsRefs(t *testing.T) {
	records := []backtestdoc.Experience{
		ruleProvenanceExperienceForTest("exp-3", 3, "review_correction"),
		ruleProvenanceExperienceForTest("exp-1", 1, "ci_failure"),
		ruleProvenanceExperienceForTest("exp-1-dup", 1, "ci_failure"),
		ruleProvenanceExperienceForTest("exp-2", 1, "revert"),
	}

	got := RuleProvenanceRefs(records)
	want := []RuleProvenance{
		{PR: 1, Outcome: "ci_failure", URL: "https://github.com/acme/billing/pull/1", ExperienceID: "exp-1"},
		{PR: 1, Outcome: "revert", URL: "https://github.com/acme/billing/pull/1", ExperienceID: "exp-2"},
		{PR: 3, Outcome: "review_correction", URL: "https://github.com/acme/billing/pull/3", ExperienceID: "exp-3"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("RuleProvenanceRefs = %#v, want %#v", got, want)
	}
}

func ruleProvenanceExperienceForTest(id string, pr int, outcome string) backtestdoc.Experience {
	return backtestdoc.Experience{
		RecordedAt: time.Date(2026, 4, pr, 10, 0, 0, 0, time.UTC),
		Record: ingestdoc.Record{
			ExperienceID: id,
			Repo:         ingestdoc.Repo{Owner: "acme", Name: "billing"},
			Action:       ingestdoc.Action{PR: pr},
			Outcome:      ingestdoc.Outcome{Kind: outcome},
			Provenance:   ingestdoc.Provenance{URLs: []string{fmt.Sprintf("https://github.com/acme/billing/pull/%d", pr)}},
		},
	}
}
