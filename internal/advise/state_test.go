package advise

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadPriorStateMissingFileReturnsZeroState(t *testing.T) {
	root := t.TempDir()

	got, stateErr := LoadPriorState(root, ".relia/reports/advisory-state.json")

	if stateErr != nil {
		t.Fatalf("LoadPriorState returned error: %v", stateErr)
	}
	if got.DiffFingerprint != "" || !got.GeneratedAt.IsZero() || got.RiskLevel != "" || got.SkipReason != "" {
		t.Fatalf("prior state = %#v, want zero value", got)
	}
}

func TestLoadPriorStateParsesMetadataRisk(t *testing.T) {
	root := t.TempDir()
	writePriorState(t, root, ".relia/reports/advisory-state.json", `{
  "diff_fingerprint": "sha256:abc",
  "skip_reason": "reassess_debounce_window",
  "metadata": {
    "generated_at": "2026-07-02T08:00:00Z",
    "risk_level": "match_high"
  },
  "assessment": {
    "risk_level": "no_coverage"
  }
}`)

	got, stateErr := LoadPriorState(root, ".relia/reports/advisory-state.json")

	if stateErr != nil {
		t.Fatalf("LoadPriorState returned error: %v", stateErr)
	}
	if got.DiffFingerprint != "sha256:abc" {
		t.Fatalf("DiffFingerprint = %q", got.DiffFingerprint)
	}
	if got.SkipReason != "reassess_debounce_window" {
		t.Fatalf("SkipReason = %q", got.SkipReason)
	}
	if got.RiskLevel != "match_high" {
		t.Fatalf("RiskLevel = %q", got.RiskLevel)
	}
	if want := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC); !got.GeneratedAt.Equal(want) {
		t.Fatalf("GeneratedAt = %s, want %s", got.GeneratedAt, want)
	}
}

func TestLoadPriorStateFallsBackToAssessmentRisk(t *testing.T) {
	root := t.TempDir()
	writePriorState(t, root, ".relia/reports/advisory-state.json", `{
  "diff_fingerprint": "sha256:abc",
  "assessment": {
    "risk_level": "no_coverage"
  },
  "metadata": {
    "generated_at": "not-a-time"
  }
}`)

	got, stateErr := LoadPriorState(root, ".relia/reports/advisory-state.json")

	if stateErr != nil {
		t.Fatalf("LoadPriorState returned error: %v", stateErr)
	}
	if got.RiskLevel != "no_coverage" {
		t.Fatalf("RiskLevel = %q", got.RiskLevel)
	}
	if !got.GeneratedAt.IsZero() {
		t.Fatalf("GeneratedAt = %s, want zero value", got.GeneratedAt)
	}
}

func TestLoadPriorStateRejectsInvalidPath(t *testing.T) {
	_, stateErr := LoadPriorState(t.TempDir(), "../state.json")

	if stateErr == nil {
		t.Fatal("expected invalid path error")
	}
	if stateErr.Kind != StateErrorUsage {
		t.Fatalf("Kind = %q", stateErr.Kind)
	}
	if stateErr.Message != "advise --state must be repo-relative" {
		t.Fatalf("Message = %q", stateErr.Message)
	}
}

func TestLoadPriorStateRejectsInvalidJSON(t *testing.T) {
	root := t.TempDir()
	writePriorState(t, root, ".relia/reports/advisory-state.json", `{`)

	_, stateErr := LoadPriorState(root, ".relia/reports/advisory-state.json")

	if stateErr == nil {
		t.Fatal("expected invalid JSON error")
	}
	if stateErr.Kind != StateErrorArtifactContract {
		t.Fatalf("Kind = %q", stateErr.Kind)
	}
	if stateErr.Ref != ".relia/reports/advisory-state.json" {
		t.Fatalf("Ref = %q", stateErr.Ref)
	}
}

func writePriorState(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
