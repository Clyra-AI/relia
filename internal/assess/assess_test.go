package assess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	resultdoc "github.com/Clyra-AI/relia/internal/result"
)

func TestBuildRiskAssessmentSortsMatchesAndReportsHighRisk(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "packages", "billing"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "packages", "billing", "invoice.py"), []byte("def rollover_day(): pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	assessment, commandErr := BuildRiskAssessment(
		root,
		"change.diff",
		[]byte("diff content"),
		[]string{"packages/billing/invoice.py"},
		[]Rule{
			{
				ID:         "billing-medium",
				Kind:       "avoid",
				Confidence: 0.61,
				ScopePaths: []string{"packages/billing/"},
				Citations:  []RuleCitation{{URL: "https://github.com/acme/billing-service/pull/61", PR: 61, Outcome: "ci_failure"}},
				Path:       "memory/rules/billing-medium.yaml",
			},
			{
				ID:         "billing-high",
				Kind:       "avoid",
				Confidence: 0.86,
				ScopePaths: []string{"packages/billing/"},
				Citations:  []RuleCitation{{URL: "https://github.com/acme/billing-service/pull/86", PR: 86, Outcome: "review_correction"}},
				Path:       "memory/rules/billing-high.yaml",
			},
		},
		testOptions(),
	)

	if commandErr != nil {
		t.Fatal(commandErr)
	}
	if assessment.RiskLevel != "match_high" {
		t.Fatalf("risk level = %q, want match_high", assessment.RiskLevel)
	}
	if len(assessment.Matches) != 2 || assessment.Matches[0].RuleID != "billing-high" || assessment.Matches[1].RuleID != "billing-medium" {
		t.Fatalf("matches = %#v", assessment.Matches)
	}
	if got := strings.Join(assessment.Citations, ","); got != "https://github.com/acme/billing-service/pull/61,https://github.com/acme/billing-service/pull/86" {
		t.Fatalf("citations = %#v", assessment.Citations)
	}
	if assessment.Metadata["repo_relative_paths_only"] != true {
		t.Fatalf("metadata = %#v", assessment.Metadata)
	}
	if fingerprint, _ := assessment.Metadata["diff_fingerprint"].(string); !strings.HasPrefix(fingerprint, "sha256:") {
		t.Fatalf("diff_fingerprint = %q, want sha256 prefix", fingerprint)
	}
	if !strings.HasPrefix(assessment.AssessmentID, "assess_") || len(assessment.AssessmentID) != len("assess_")+12 {
		t.Fatalf("assessment_id = %q", assessment.AssessmentID)
	}
}

func TestServedRuleDataRejectsPlaybookWithoutPositiveCitation(t *testing.T) {
	_, commandErr := ServedRuleData([]Rule{
		{
			ID:         "billing-playbook",
			Kind:       "playbook",
			Confidence: 0.9,
			ScopePaths: []string{"packages/billing/"},
			Citations:  []RuleCitation{{URL: "https://github.com/acme/billing-service/pull/142", PR: 142, Outcome: "ci_failure"}},
			Path:       "memory/rules/billing-playbook.yaml",
		},
	}, testOptions())

	if commandErr == nil {
		t.Fatalf("expected provenance error")
	}
	if commandErr.Type != "provenance_integrity_failed" || !strings.Contains(commandErr.Message, "citation URLs") {
		t.Fatalf("error = %#v", commandErr)
	}
}

func TestRuleCitationURLsTrimAndDeduplicateWhitespace(t *testing.T) {
	urls := RuleCitationURLs([]RuleCitation{
		{URL: " https://github.com/acme/billing-service/pull/142 ", PR: 142, Outcome: "ci_failure"},
		{URL: "https://github.com/acme/billing-service/pull/142", PR: 142, Outcome: "review_correction"},
	})

	if len(urls) != 1 || urls[0] != "https://github.com/acme/billing-service/pull/142" {
		t.Fatalf("urls = %#v", urls)
	}
}

func testOptions() Options {
	return Options{
		SchemaVersion: "1.0",
		ProvenanceIntegrityError: func(message string, ref string) *resultdoc.CommandError {
			return &resultdoc.CommandError{Type: "provenance_integrity_failed", Message: message, ExitCode: 9, Ref: ref}
		},
	}
}
