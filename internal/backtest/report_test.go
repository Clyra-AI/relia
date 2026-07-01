package backtest

import (
	"strings"
	"testing"
	"time"

	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

func TestReportRepoIDUsesFirstCompleteRepo(t *testing.T) {
	id := ReportRepoID([]Experience{
		{Record: ingestdoc.Record{Repo: ingestdoc.Repo{Owner: "acme", Name: "billing"}}},
	})

	if id != "acme/billing" {
		t.Fatalf("repo id = %q, want acme/billing", id)
	}
	if id := ReportRepoID(nil); id != "" {
		t.Fatalf("empty repo id = %q, want empty", id)
	}
	if id := ReportRepoID([]Experience{{Record: ingestdoc.Record{Repo: ingestdoc.Repo{Owner: "acme"}}}}); id != "" {
		t.Fatalf("incomplete repo id = %q, want empty", id)
	}
}

func TestReportIngestFreshnessMetadataUsesLatestIngestAndMaxMergedPRs(t *testing.T) {
	records := []Experience{
		{Record: ingestdoc.Record{Metadata: map[string]any{
			"last_ingest_at":               "2026-06-01T00:00:00Z",
			"merged_prs_since_last_ingest": 7,
		}}},
		{Record: ingestdoc.Record{Metadata: map[string]any{
			"last_ingest_at":               "2026-06-20T00:00:00Z",
			"merged_prs_since_last_ingest": 3,
		}}},
		{Record: ingestdoc.Record{Metadata: map[string]any{
			"last_ingest_at":               "2026-06-20T00:00:00Z",
			"merged_prs_since_last_ingest": 5,
		}}},
	}

	ingestedAt, mergedSinceIngest, hasIngest, hasMerged := ReportIngestFreshnessMetadata(records)

	if !hasIngest || !hasMerged {
		t.Fatalf("freshness presence = ingest:%v merged:%v, want both", hasIngest, hasMerged)
	}
	if ingestedAt.Format(time.RFC3339) != "2026-06-20T00:00:00Z" {
		t.Fatalf("ingestedAt = %s, want latest timestamp", ingestedAt.Format(time.RFC3339))
	}
	if mergedSinceIngest != 5 {
		t.Fatalf("mergedSinceIngest = %d, want max for latest timestamp", mergedSinceIngest)
	}
}

func TestReportIngestFreshnessMetadataReportsMissingMergedCount(t *testing.T) {
	records := []Experience{
		{Record: ingestdoc.Record{Metadata: map[string]any{
			"last_ingest_at": "2026-06-20T00:00:00Z",
		}}},
	}

	ingestedAt, mergedSinceIngest, hasIngest, hasMerged := ReportIngestFreshnessMetadata(records)

	if !hasIngest || hasMerged {
		t.Fatalf("freshness presence = ingest:%v merged:%v, want ingest only", hasIngest, hasMerged)
	}
	if ingestedAt.Format(time.RFC3339) != "2026-06-20T00:00:00Z" || mergedSinceIngest != 0 {
		t.Fatalf("freshness = %s/%d, want timestamp and zero merged count", ingestedAt.Format(time.RFC3339), mergedSinceIngest)
	}
}

func TestReportIngestFreshnessMetadataIgnoresInvalidValues(t *testing.T) {
	records := []Experience{
		{Record: ingestdoc.Record{Metadata: map[string]any{
			"last_ingest_at":               "not-time",
			"merged_prs_since_last_ingest": 99,
		}}},
		{Record: ingestdoc.Record{Metadata: map[string]any{
			"merged_prs_since_last_ingest": 2,
		}}},
	}

	_, _, hasIngest, hasMerged := ReportIngestFreshnessMetadata(records)

	if hasIngest || hasMerged {
		t.Fatalf("freshness presence = ingest:%v merged:%v, want neither", hasIngest, hasMerged)
	}
}

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
