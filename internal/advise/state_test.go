package advise

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	assessdoc "github.com/Clyra-AI/relia/internal/assess"
	configdoc "github.com/Clyra-AI/relia/internal/config"
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

func TestLoadForwardBaselineReadsSavedERRBaseline(t *testing.T) {
	root := t.TempDir()
	writePriorState(t, root, ".relia/baselines/error-recurrence-baseline.json", `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.2143,
  "report_id": "backtest_demo"
}`)

	got, stateErr := LoadForwardBaseline(root, ".relia/baselines/error-recurrence-baseline.json")

	if stateErr != nil {
		t.Fatalf("LoadForwardBaseline returned error: %v", stateErr)
	}
	if got.Status != "current" || got.Path != ".relia/baselines/error-recurrence-baseline.json" || !got.HasHeadlineERR || got.HeadlineERR != 0.2143 {
		t.Fatalf("baseline = %#v", got)
	}
}

func TestLoadForwardBaselineMissingIsNonBlockingSignal(t *testing.T) {
	got, stateErr := LoadForwardBaseline(t.TempDir(), ".relia/baselines/error-recurrence-baseline.json")

	if stateErr != nil {
		t.Fatalf("LoadForwardBaseline returned error: %v", stateErr)
	}
	if got.Status != "missing" || got.HasHeadlineERR {
		t.Fatalf("baseline = %#v", got)
	}
}

func TestStateDocumentBuildsAdvisoryState(t *testing.T) {
	generatedAt := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	assessment := assessdoc.RiskAssessment{
		ObjectType:    "relia.risk_assessment",
		SchemaVersion: "1.0",
		RiskLevel:     "no_coverage",
		Metadata:      map[string]any{},
	}
	settings := configdoc.AdviseSettings{
		Enabled:                 true,
		MaxCommentsPerPR:        1,
		UpdateInPlace:           true,
		ReassessDebounceMinutes: 15,
		MinConfidence:           0.7,
	}

	forwardSignal := BuildForwardSignal("1.0", "changes.diff", assessment, settings, "sha256:current", ForwardBaseline{
		Status:         "current",
		Path:           ".relia/baselines/error-recurrence-baseline.json",
		HeadlineERR:    0.2143,
		HasHeadlineERR: true,
	}, true, "", generatedAt)

	got := StateDocument("1.0", "changes.diff", assessment, settings, "sha256:current", PriorState{}, true, "", generatedAt, forwardSignal)

	if got["object_type"] != "relia.advisory_state" {
		t.Fatalf("object_type = %q", got["object_type"])
	}
	if got["schema_version"] != "1.0" || got["input_path"] != "changes.diff" {
		t.Fatalf("schema/input = %#v", got)
	}
	if got["diff_fingerprint"] != "sha256:current" || got["previous_diff_fingerprint"] != "" {
		t.Fatalf("fingerprints = %#v/%#v", got["diff_fingerprint"], got["previous_diff_fingerprint"])
	}
	metadata := got["metadata"].(map[string]any)
	if metadata["generated_at"] != "2026-07-02T08:00:00Z" {
		t.Fatalf("metadata.generated_at = %#v", metadata["generated_at"])
	}
	if metadata["github_api_required_later"] != true || metadata["risk_level"] != "no_coverage" {
		t.Fatalf("metadata = %#v", metadata)
	}
	strategy := got["comment_strategy"].(map[string]any)
	if strategy["comment_marker"] != "relia-advisory:v1" || strategy["max_comments_per_pr"] != 1 {
		t.Fatalf("comment_strategy = %#v", strategy)
	}
	stateForwardSignal := got["forward_signal"].(map[string]any)
	if stateForwardSignal["object_type"] != "relia.forward_signal" ||
		stateForwardSignal["risk_level"] != "no_coverage" ||
		stateForwardSignal["comment_action"] != "publish" {
		t.Fatalf("forward_signal = %#v", stateForwardSignal)
	}
	baseline := stateForwardSignal["baseline"].(map[string]any)
	if baseline["status"] != "current" || baseline["headline_err"] != 0.2143 {
		t.Fatalf("forward signal baseline = %#v", baseline)
	}
}

func TestStateDocumentPreservesPriorFingerprintDuringDebounce(t *testing.T) {
	generatedAt := time.Date(2026, 7, 2, 8, 30, 0, 0, time.UTC)
	previousAt := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	assessment := assessdoc.RiskAssessment{RiskLevel: "match_high", Metadata: map[string]any{}}

	got := StateDocument("1.0", "changes.diff", assessment, configdoc.AdviseSettings{}, "sha256:current", PriorState{
		DiffFingerprint: "sha256:previous",
		GeneratedAt:     previousAt,
		RiskLevel:       "match_high",
	}, false, "reassess_debounce_window", generatedAt)

	if got["diff_fingerprint"] != "sha256:previous" {
		t.Fatalf("diff_fingerprint = %#v", got["diff_fingerprint"])
	}
	metadata := got["metadata"].(map[string]any)
	if metadata["generated_at"] != "2026-07-02T08:00:00Z" {
		t.Fatalf("metadata.generated_at = %#v", metadata["generated_at"])
	}
	if metadata["debounced_diff_fingerprint"] != "sha256:current" || metadata["debounced_at"] != "2026-07-02T08:30:00Z" {
		t.Fatalf("debounce metadata = %#v", metadata)
	}
}

func TestCommentDecisionSkipsUnchangedFingerprintBeforeDebounce(t *testing.T) {
	assessment := assessdoc.RiskAssessment{RiskLevel: "match_high", Metadata: map[string]any{"max_avoid_confidence": 0.86}}
	settings := configdoc.AdviseSettings{
		Enabled:                 true,
		MaxCommentsPerPR:        1,
		ReassessDebounceMinutes: 10,
		MinConfidence:           0.6,
	}
	now := time.Date(2026, 7, 2, 8, 5, 0, 0, time.UTC)

	shouldComment, skipReason := CommentDecision(settings, assessment, "sha256:same", PriorState{
		DiffFingerprint: "sha256:same",
		GeneratedAt:     now.Add(-time.Minute),
		RiskLevel:       "match_high",
	}, now)

	if shouldComment || skipReason != "unchanged_diff_fingerprint" {
		t.Fatalf("CommentDecision = (%v, %q), want unchanged skip before debounce", shouldComment, skipReason)
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
