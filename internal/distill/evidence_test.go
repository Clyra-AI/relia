package distill

import (
	"testing"
	"time"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

func TestEvidenceClassificationSplitsFailurePositiveAndHeld(t *testing.T) {
	records := []backtestdoc.Experience{
		experienceForTest("ci_failure", "exp-1", "2026-04-01T10:00:00Z"),
		experienceForTest("fix_held", "exp-2", "2026-04-02T10:00:00Z"),
		experienceForTest("merged_clean", "exp-3", "2026-04-03T10:00:00Z"),
		experienceForTest("noop", "exp-4", "2026-04-04T10:00:00Z"),
	}

	if got := len(FailureEvidence(records)); got != 1 {
		t.Fatalf("FailureEvidence count = %d, want 1", got)
	}
	if got := len(PositiveEvidence(records)); got != 2 {
		t.Fatalf("PositiveEvidence count = %d, want 2", got)
	}
	held := HeldEvidence(records)
	if len(held) != 1 || held[0].Record.ExperienceID != "exp-2" {
		t.Fatalf("HeldEvidence = %#v, want exp-2 only", held)
	}
}

func TestAvoidContradictionsIgnoreOlderPositiveEvidence(t *testing.T) {
	failures := []backtestdoc.Experience{
		experienceForTest("ci_failure", "failure", "2026-04-10T10:00:00Z"),
	}
	positives := []backtestdoc.Experience{
		experienceForTest("merged_clean", "older-positive", "2026-04-01T10:00:00Z"),
	}
	if got := AvoidContradictions(failures, positives); got != 0 {
		t.Fatalf("older positive evidence counted as contradiction: %d", got)
	}
	positives = append(positives, experienceForTest("fix_held", "newer-positive", "2026-04-12T10:00:00Z"))
	if got := AvoidContradictions(failures, positives); got != 1 {
		t.Fatalf("later positive evidence contradictions = %d, want 1", got)
	}
}

func TestPlaybookContradictionsCountLaterFailures(t *testing.T) {
	positives := []backtestdoc.Experience{
		experienceForTest("fix_held", "held", "2026-04-10T10:00:00Z"),
	}
	failures := []backtestdoc.Experience{
		experienceForTest("ci_failure", "older-failure", "2026-04-01T10:00:00Z"),
		experienceForTest("review_correction", "newer-failure", "2026-04-12T10:00:00Z"),
	}
	if got := PlaybookContradictions(positives, failures); got != 1 {
		t.Fatalf("PlaybookContradictions = %d, want 1", got)
	}
}

func TestAllEvidenceDiscountedUsesHeuristicsAndBounds(t *testing.T) {
	records := []backtestdoc.Experience{
		experienceForTest("ci_failure", "heuristic-flake", "2026-04-01T10:00:00Z"),
		experienceWithDiscountForTest("review_correction", "clamped-flake", "2026-04-02T10:00:00Z", 2),
	}
	if !AllEvidenceDiscounted(records, map[string]string{"heuristic-flake": "duplicate support"}) {
		t.Fatal("AllEvidenceDiscounted returned false for heuristic/clamped flakes")
	}
	records = append(records, experienceWithDiscountForTest("ci_failure", "real-failure", "2026-04-03T10:00:00Z", 0.2))
	if AllEvidenceDiscounted(records, map[string]string{"heuristic-flake": "duplicate support"}) {
		t.Fatal("AllEvidenceDiscounted returned true with non-discounted evidence")
	}
}

func experienceForTest(kind string, id string, recordedAt string) backtestdoc.Experience {
	return experienceWithDiscountForTest(kind, id, recordedAt, 0)
}

func experienceWithDiscountForTest(kind string, id string, recordedAt string, flakeDiscount float64) backtestdoc.Experience {
	parsed, err := time.Parse(time.RFC3339, recordedAt)
	if err != nil {
		panic(err)
	}
	return backtestdoc.Experience{
		RecordedAt: parsed,
		Record: ingestdoc.Record{
			ExperienceID:  id,
			FlakeDiscount: flakeDiscount,
			Outcome:       ingestdoc.Outcome{Kind: kind},
		},
	}
}
