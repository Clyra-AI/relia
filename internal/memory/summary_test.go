package memory

import (
	"strings"
	"testing"

	"github.com/Clyra-AI/relia/internal/yamlmini"
)

func TestRuleProvenanceEntriesSortsByPRAndExperience(t *testing.T) {
	document, err := yamlmini.ParseDocument(`provenance:
  - pr: 42
    outcome: ci_failure
    url: https://github.com/acme/repo/pull/42
    experience_id: exp_b
  - pr: 41
    outcome: merged_clean
    url: https://github.com/acme/repo/pull/41
    experience_id: exp_a
  - pr: 42
    outcome: review_correction
    url: https://github.com/acme/repo/pull/42
    experience_id: exp_a
`)
	if err != nil {
		t.Fatal(err)
	}

	entries := RuleProvenanceEntries(document)

	if len(entries) != 3 {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].PR != 41 || entries[1].ExperienceID != "exp_a" || entries[2].ExperienceID != "exp_b" {
		t.Fatalf("entries not sorted: %#v", entries)
	}
}

func TestRenderMarkdownSplitsStrongAndWeakMemory(t *testing.T) {
	markdown := RenderMarkdown([]RuleSummary{
		{
			ID:              "active-rule",
			Kind:            "avoid",
			Status:          "active",
			Statement:       "Avoid direct UTC clocks.",
			Confidence:      "0.86",
			ConfidenceLabel: "high",
			EvidenceCount:   "2",
			Contradictions:  "0",
			ReviewLabel:     "accepted",
			StatementOrigin: "human_authored",
			Path:            "memory/rules/active-rule.yaml",
			Provenance:      []RuleProvenance{{PR: 142, URL: "https://github.com/acme/repo/pull/142"}},
		},
		{
			ID:              "candidate-rule",
			Kind:            "playbook",
			Status:          "candidate",
			Statement:       "Use the approved clock fixture.",
			Confidence:      "0.64",
			ConfidenceLabel: "medium",
			EvidenceCount:   "1",
			Contradictions:  "0",
			ReviewLabel:     "suggested",
			StatementOrigin: "cluster_summary",
			Path:            "memory/rules/candidate-rule.yaml",
		},
	})

	for _, want := range []string{
		"## Strong Memory",
		"### Active",
		"#### active-rule",
		"[PR #142](https://github.com/acme/repo/pull/142)",
		"## Weak Memory",
		"### Candidate",
		"#### candidate-rule",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
	if strings.Index(markdown, "## Strong Memory") > strings.Index(markdown, "## Weak Memory") {
		t.Fatalf("strong memory should render before weak memory:\n%s", markdown)
	}
}

func TestStatusCounts(t *testing.T) {
	counts := StatusCounts([]RuleSummary{
		{Status: "active"},
		{Status: "candidate"},
		{Status: "active"},
	})

	if counts["active"] != 2 || counts["candidate"] != 1 {
		t.Fatalf("counts = %#v", counts)
	}
}
