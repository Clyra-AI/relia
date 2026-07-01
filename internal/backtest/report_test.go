package backtest

import (
	"strings"
	"testing"
	"time"
)

func TestBuildTopRepeatedMistakesAggregatesByMatchedSignature(t *testing.T) {
	mistakes := BuildTopRepeatedMistakes([]RecurrencePair{
		{
			CurrentExperienceID: "exp-2",
			PriorExperienceID:   "exp-1",
			CurrentPR:           12,
			PriorPR:             11,
			CurrentURL:          "https://example.test/pr/12",
			PriorURL:            "https://example.test/pr/11",
			SignatureID:         "raw-sig",
			MatchedSignatureID:  "class_key:test:case",
			Refs:                []string{".relia/experiences/a.jsonl:1", ".relia/experiences/a.jsonl:2"},
		},
		{
			CurrentExperienceID: "exp-3",
			PriorExperienceID:   "exp-2",
			CurrentPR:           13,
			PriorPR:             12,
			CurrentURL:          "https://example.test/pr/13",
			PriorURL:            "https://example.test/pr/12",
			SignatureID:         "class_key:test:case",
			Refs:                []string{".relia/experiences/a.jsonl:2", ".relia/experiences/a.jsonl:3"},
		},
	})

	if len(mistakes) != 1 {
		t.Fatalf("mistakes = %#v, want one aggregate", mistakes)
	}
	if mistakes[0].Rank != 1 || mistakes[0].RepeatCount != 2 {
		t.Fatalf("aggregate = %#v, want rank 1 repeat count 2", mistakes[0])
	}
	if len(mistakes[0].PRs) != 3 || mistakes[0].PRs[0] != 11 || mistakes[0].PRs[2] != 13 {
		t.Fatalf("aggregate PRs = %#v", mistakes[0].PRs)
	}
	if len(mistakes[0].ExperienceIDs) != 3 {
		t.Fatalf("aggregate experience ids = %#v", mistakes[0].ExperienceIDs)
	}
}

func TestBuildReportDiagnosticsIncludesOptionalFindings(t *testing.T) {
	diagnostics := BuildReportDiagnostics(
		RecurrenceSummary{
			PossibleRecurrenceCount:   1,
			FlakeDiscountedCount:      2,
			AttributionUncertainCount: 3,
		},
		BaselineComparison{
			Status: "stale",
			Path:   ".relia/baselines/error-recurrence-baseline.json",
			Stale:  true,
			Reason: "Saved baseline source artifact digest differs from the current backtest inputs.",
		},
		[]string{".relia/experiences/2026.jsonl"},
	)

	seen := map[string]bool{}
	for _, diagnostic := range diagnostics {
		seen[diagnostic.Type] = true
	}
	for _, want := range []string{
		"memory_source_verified",
		"possible_recurrences_excluded",
		"flake_discounts_visible",
		"uncertain_attribution_excluded",
		"stale_baseline",
	} {
		if !seen[want] {
			t.Fatalf("diagnostics missing %q: %#v", want, diagnostics)
		}
	}
}

func TestBuildReportBadgeAtComputesFreshness(t *testing.T) {
	now := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	report := RecurrenceReport{
		ReportID:    "backtest_fresh",
		Summary:     RecurrenceSummary{HeadlineERRPercent: "4.1%"},
		HeadlineERR: 0.041,
		Metadata: map[string]any{
			"last_ingest_at":               "2026-06-20T00:00:00Z",
			"merged_prs_since_last_ingest": 0,
		},
	}

	badge := BuildReportBadgeAt(report, now)
	if badge.Status != "current" || badge.Stale || badge.Message != "ERR 4.1%" || badge.Color != "brightgreen" {
		t.Fatalf("fresh badge = %#v, want current", badge)
	}
	if !strings.Contains(badge.Reason, "ingest metadata") {
		t.Fatalf("fresh badge reason = %q", badge.Reason)
	}
}
