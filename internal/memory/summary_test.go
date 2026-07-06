package memory

import (
	"os"
	"path/filepath"
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

func TestLoadRuleSummariesSortsAndLoadsValidatedRules(t *testing.T) {
	root := t.TempDir()
	writeValidationFixture(t, root, validMemoryRuleYAML())
	candidate := strings.Replace(validMemoryRuleYAML(), "id: rule-1", "id: rule-2", 1)
	candidate = strings.Replace(candidate, "status: active", "status: candidate", 1)
	candidate = strings.Replace(candidate, "label: accepted", "label: suggested", 1)
	if err := os.WriteFile(filepath.Join(root, "memory", "rules", "candidate.yaml"), []byte(candidate), 0o644); err != nil {
		t.Fatal(err)
	}

	summaries, commandErr := LoadRuleSummaries(root, testValidationOptions())

	if commandErr != nil {
		t.Fatal(commandErr)
	}
	if len(summaries) != 2 {
		t.Fatalf("summaries = %#v", summaries)
	}
	if summaries[0].ID != "rule-1" || summaries[1].ID != "rule-2" {
		t.Fatalf("summaries not sorted by status rank: %#v", summaries)
	}
	if summaries[0].Path != "memory/rules/rule.yaml" {
		t.Fatalf("path = %q", summaries[0].Path)
	}
	if len(summaries[0].Provenance) != 1 || summaries[0].Provenance[0].PR != 1 {
		t.Fatalf("provenance = %#v", summaries[0].Provenance)
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

func TestRenderMarkdownShowsManagedMetadataReceiptsAndLifecycleReasons(t *testing.T) {
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
			Provenance: []RuleProvenance{
				{PR: 142, Outcome: "ci_failure", URL: "https://github.com/acme/repo/pull/142", ExperienceID: "exp_0142"},
			},
		},
		{
			ID:              "contradicted-rule",
			Kind:            "avoid",
			Status:          "contradicted",
			Statement:       "Avoid blind schema regeneration.",
			Confidence:      "0.44",
			ConfidenceLabel: "low",
			EvidenceCount:   "4",
			Contradictions:  "2",
			ReviewLabel:     "needs_user_input",
			StatementOrigin: "cluster_summary",
			LifecycleReason: "later clean merges contradicted the avoided pattern",
			Path:            "memory/rules/contradicted-rule.yaml",
			Provenance: []RuleProvenance{
				{PR: 282, Outcome: "merged_clean", URL: "https://github.com/acme/repo/pull/282", ExperienceID: "exp_0282"},
			},
		},
	}, RenderOptions{SchemaVersion: "1.0", ReliaVersion: "0.0.0-dev"})

	for _, want := range []string{
		"<!-- relia:memory-page generated; schema_version=1.0; relia_version=0.0.0-dev; source=memory/rules -->",
		"- schema version: `1.0`",
		"- relia version: `0.0.0-dev`",
		"| active | 1 | served as strong memory |",
		"| contradicted | 1 | visible only; not served |",
		"- lifecycle: `contradicted` (visible only; not served)",
		"- lifecycle reason: later clean merges contradicted the avoided pattern",
		"  - [PR #282](https://github.com/acme/repo/pull/282) - outcome `merged_clean`, experience `exp_0282`",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
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
