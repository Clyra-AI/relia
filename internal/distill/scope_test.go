package distill

import (
	"fmt"
	"testing"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

func TestNormalizedRepoPathsRejectsUnsafeAndSortsUnique(t *testing.T) {
	got := NormalizedRepoPaths([]string{" ./cmd/relia/main.go ", "cmd/relia/main.go", "../secret", "/abs/path", "internal/distill/rule.go"})
	want := []string{"cmd/relia/main.go", "internal/distill/rule.go"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("NormalizedRepoPaths = %#v, want %#v", got, want)
	}
}

func TestScopePathsPrefersNonTestTopCounts(t *testing.T) {
	records := []backtestdoc.Experience{
		scopeExperienceForTest("exp-1", []string{"cmd/relia/main.go", "cmd/relia/main_test.go"}, ""),
		scopeExperienceForTest("exp-2", []string{"cmd/relia/main.go"}, ""),
		scopeExperienceForTest("exp-3", []string{"internal/distill/rule.go"}, ""),
		scopeExperienceForTest("exp-4", []string{"internal/distill/rule.go"}, ""),
		scopeExperienceForTest("exp-5", []string{"internal/distill/confidence.go"}, ""),
	}

	got := ScopePaths(records)
	want := []string{"cmd/relia/main.go", "internal/distill/rule.go", "internal/distill/confidence.go"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("ScopePaths = %#v, want %#v", got, want)
	}
}

func TestScopePathsFallsBackToTestPaths(t *testing.T) {
	records := []backtestdoc.Experience{
		scopeExperienceForTest("exp-1", []string{"cmd/relia/main_test.go"}, ""),
	}

	got := ScopePaths(records)
	want := []string{"cmd/relia/main_test.go"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("ScopePaths = %#v, want %#v", got, want)
	}
}

func TestScopeSignalsUsesClusterSignalThenRecordSignals(t *testing.T) {
	records := []backtestdoc.Experience{
		scopeExperienceForTest("exp-1", nil, "go test"),
		scopeExperienceForTest("exp-2", nil, "go test"),
		scopeExperienceForTest("exp-3", nil, "go vet"),
	}

	got := ScopeSignals(Cluster{Signal: "go vet"}, records)
	want := []string{"go test", "go vet"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("ScopeSignals = %#v, want %#v", got, want)
	}
}

func scopeExperienceForTest(id string, paths []string, checkName string) backtestdoc.Experience {
	metadata := map[string]any{}
	if checkName != "" {
		metadata["signature"] = map[string]any{"check_name": checkName}
	}
	return backtestdoc.Experience{
		Record: ingestdoc.Record{
			ExperienceID: id,
			Context:      ingestdoc.Context{Paths: paths},
			Outcome:      ingestdoc.Outcome{Kind: "ci_failure", Signature: ingestdoc.Signature{SignatureID: "sig-" + id}},
			Metadata:     metadata,
		},
	}
}
