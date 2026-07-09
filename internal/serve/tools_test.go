package serve

import (
	"fmt"
	"testing"

	assessdoc "github.com/Clyra-AI/relia/internal/assess"
	resultdoc "github.com/Clyra-AI/relia/internal/result"
)

func TestBuildCoverageResultReportsExperienceDensity(t *testing.T) {
	rules := []assessdoc.Rule{
		{
			ID:                  "billing-time",
			Kind:                "avoid",
			Confidence:          0.86,
			ScopePaths:          []string{"packages/billing/"},
			EvidenceCount:       3,
			EvidenceExperiences: []string{"exp_0142", "exp_0187", "exp_0203"},
			Citations:           []assessdoc.RuleCitation{{URL: "https://github.com/acme/billing-service/pull/142", PR: 142, Outcome: "ci_failure"}},
			Path:                "memory/rules/billing-time.yaml",
		},
	}

	result, commandErr := BuildCoverageResult(t.TempDir(), []string{"packages/billing/invoice.py", "packages/search/query.py"}, rules, assessdoc.Options{
		SchemaVersion: "1.0",
		ProvenanceIntegrityError: func(message string, ref string) *resultdoc.CommandError {
			return &resultdoc.CommandError{Type: "provenance_integrity_failed", Message: message, ExitCode: 9, Ref: ref}
		},
	})

	if commandErr != nil {
		t.Fatal(commandErr)
	}
	entries := result["entries"].([]map[string]any)
	if len(entries) != 2 {
		t.Fatalf("entries = %#v", entries)
	}
	byPath := map[string]map[string]any{}
	for _, entry := range entries {
		byPath[entry["path"].(string)] = entry
	}
	billing := byPath["packages/billing/invoice.py"]
	if billing["coverage"] != "covered_risky" ||
		billing["experience_density"] != float64(3) ||
		billing["evidence_count"] != 3 ||
		fmt.Sprint(billing["experience_ids"]) != fmt.Sprint([]string{"exp_0142", "exp_0187", "exp_0203"}) {
		t.Fatalf("billing entry = %#v", billing)
	}
	search := byPath["packages/search/query.py"]
	if search["coverage"] != "no_coverage" ||
		search["experience_density"] != float64(0) ||
		search["evidence_count"] != 0 {
		t.Fatalf("search entry = %#v", search)
	}
	summary := result["summary"].(map[string]any)
	if summary["experience_density"] != float64(1.5) ||
		summary["covered_path_count"] != 1 ||
		summary["no_coverage_count"] != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}
