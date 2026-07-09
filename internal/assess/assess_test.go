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
				ID:                  "billing-medium",
				Kind:                "avoid",
				Confidence:          0.61,
				ScopePaths:          []string{"packages/billing/"},
				EvidenceCount:       2,
				EvidenceExperiences: []string{"exp_0060", "exp_0061"},
				Citations:           []RuleCitation{{URL: "https://github.com/acme/billing-service/pull/61", PR: 61, Outcome: "ci_failure"}},
				Path:                "memory/rules/billing-medium.yaml",
			},
			{
				ID:                  "billing-high",
				Kind:                "avoid",
				Confidence:          0.86,
				ScopePaths:          []string{"packages/billing/"},
				EvidenceCount:       3,
				EvidenceExperiences: []string{"exp_0084", "exp_0085", "exp_0086"},
				Citations:           []RuleCitation{{URL: "https://github.com/acme/billing-service/pull/86", PR: 86, Outcome: "review_correction"}},
				Path:                "memory/rules/billing-high.yaml",
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
	coverageStats := assessment.Metadata["coverage_stats"].(map[string]any)
	if coverageStats["coverage"] != "covered_risky" ||
		coverageStats["touched_path_count"] != 1 ||
		coverageStats["covered_path_count"] != 1 ||
		coverageStats["no_coverage_path_count"] != 0 ||
		coverageStats["matched_rule_count"] != 2 ||
		coverageStats["evidence_count"] != 5 ||
		coverageStats["experience_density"] != float64(5) {
		t.Fatalf("coverage_stats = %#v", coverageStats)
	}
	pathCoverage := assessment.Metadata["path_coverage"].([]map[string]any)
	if len(pathCoverage) != 1 ||
		pathCoverage[0]["path"] != "packages/billing/invoice.py" ||
		pathCoverage[0]["coverage"] != "covered_risky" ||
		pathCoverage[0]["evidence_count"] != 5 ||
		pathCoverage[0]["experience_density"] != float64(5) {
		t.Fatalf("path_coverage = %#v", pathCoverage)
	}
	if got := assessment.Metadata["memory_source"]; got != "active_rules_from_canonical_experience_records" {
		t.Fatalf("memory_source = %#v", got)
	}
}

func TestBuildRiskAssessmentReportsNoCoverageDensityDistinctFromCoveredClean(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "packages", "billing"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "packages", "billing", "invoice.py"), []byte("def rollover_day(): pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	assessment, commandErr := BuildRiskAssessment(
		root,
		"plan.md",
		[]byte("Update packages/search/query.py and packages/billing/invoice.py"),
		[]string{"packages/billing/invoice.py", "packages/search/query.py"},
		[]Rule{
			{
				ID:                  "billing-playbook",
				Kind:                "playbook",
				Confidence:          0.91,
				ScopePaths:          []string{"packages/billing/"},
				EvidenceCount:       2,
				EvidenceExperiences: []string{"exp_0141", "exp_0142"},
				Citations:           []RuleCitation{{URL: "https://github.com/acme/billing-service/pull/142", PR: 142, Outcome: "merged_clean"}},
				Path:                "memory/rules/billing-playbook.yaml",
			},
		},
		testOptions(),
	)

	if commandErr != nil {
		t.Fatal(commandErr)
	}
	if assessment.RiskLevel != "no_coverage" {
		t.Fatalf("risk level = %q, want no_coverage", assessment.RiskLevel)
	}
	coverageStats := assessment.Metadata["coverage_stats"].(map[string]any)
	if coverageStats["coverage"] != "no_coverage" ||
		coverageStats["covered_path_count"] != 1 ||
		coverageStats["no_coverage_path_count"] != 1 ||
		coverageStats["out_of_distribution_signal"] != true ||
		coverageStats["evidence_count"] != 2 ||
		coverageStats["experience_density"] != float64(1) {
		t.Fatalf("coverage_stats = %#v", coverageStats)
	}
	pathCoverage := assessment.Metadata["path_coverage"].([]map[string]any)
	coverageByPath := map[string]string{}
	for _, entry := range pathCoverage {
		coverageByPath[entry["path"].(string)] = entry["coverage"].(string)
	}
	if coverageByPath["packages/billing/invoice.py"] != "covered_clean" ||
		coverageByPath["packages/search/query.py"] != "no_coverage" {
		t.Fatalf("path coverage = %#v", pathCoverage)
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

func TestLoadRulesReturnsOnlyActiveAcceptedRules(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "packages", "billing"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "memory", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "packages", "billing", "invoice.py"), []byte("def rollover_day(): pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRuleForTest(t, root, "active.yaml", `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-active
kind: avoid
status: active
statement: Use the billing clock fixture instead of direct UTC calls.
scope:
  paths:
    - packages/billing/
confidence: 0.86
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0142
provenance:
  - pr: 142
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/142
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`)
	writeRuleForTest(t, root, "candidate.yaml", `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-candidate
kind: avoid
status: candidate
statement: Candidate rules must not be served.
scope:
  paths:
    - packages/billing/
confidence: 0.76
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0143
provenance:
  - pr: 143
    outcome: ci_failure
review:
  label: suggested
  statement_origin: human_authored
metadata: {}
`)

	rules, commandErr := LoadRules(root, testOptions())

	if commandErr != nil {
		t.Fatal(commandErr)
	}
	if len(rules) != 1 || rules[0].ID != "billing-active" {
		t.Fatalf("rules = %#v", rules)
	}
	if rules[0].ReviewGate != "human_review" || rules[0].ReviewDecision != "approved" {
		t.Fatalf("review normalization = gate %q decision %q", rules[0].ReviewGate, rules[0].ReviewDecision)
	}
}

func TestLoadRulesResolvesBlockScalarStatement(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "packages", "billing"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "memory", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "packages", "billing", "invoice.py"), []byte("def rollover_day(): pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRuleForTest(t, root, "active.yaml", `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-active
kind: avoid
status: active
statement: >
  Use the billing clock fixture
  instead of direct UTC calls.
scope:
  paths:
    - packages/billing/
confidence: 0.86
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0142
provenance:
  - pr: 142
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/142
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`)

	rules, commandErr := LoadRules(root, testOptions())

	if commandErr != nil {
		t.Fatal(commandErr)
	}
	if len(rules) != 1 {
		t.Fatalf("rules = %#v", rules)
	}
	if got := rules[0].Statement; got != "Use the billing clock fixture instead of direct UTC calls." {
		t.Fatalf("statement = %q", got)
	}
}

func TestLoadRulesRejectsInvalidActiveRule(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "memory", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuleForTest(t, root, "active.yaml", `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-active
kind: avoid
status: active
statement: Use the billing clock fixture instead of direct UTC calls.
scope:
  paths:
    - packages/missing/
confidence: 0.86
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0142
provenance:
  - pr: 142
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/142
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`)

	_, commandErr := LoadRules(root, testOptions())

	if commandErr == nil {
		t.Fatalf("expected artifact contract error")
	}
	if commandErr.Type != "artifact_contract_validation_failed" || !strings.Contains(commandErr.Message, "scope path does not exist") {
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
		ArtifactContractError: func(message string, ref string) *resultdoc.CommandError {
			return &resultdoc.CommandError{Type: "artifact_contract_validation_failed", Message: message, ExitCode: 4, Ref: ref}
		},
		InternalError: func(message string, err error) *resultdoc.CommandError {
			if err != nil {
				message += ": " + err.Error()
			}
			return &resultdoc.CommandError{Type: "internal", Message: message, ExitCode: 1}
		},
		ProvenanceIntegrityError: func(message string, ref string) *resultdoc.CommandError {
			return &resultdoc.CommandError{Type: "provenance_integrity_failed", Message: message, ExitCode: 9, Ref: ref}
		},
	}
}

func writeRuleForTest(t *testing.T, root string, name string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "memory", "rules", name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
