package backtest

import (
	"crypto/sha256"
	"fmt"
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

func TestSortAndWindowRecordsUseStableRecordedAtBounds(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	records := []Experience{
		{RecordedAt: end, Record: ingestdoc.Record{ExperienceID: "exp-c"}},
		{RecordedAt: old, Record: ingestdoc.Record{ExperienceID: "exp-a"}},
		{RecordedAt: mid, Record: ingestdoc.Record{ExperienceID: "exp-b"}},
	}

	SortExperiences(records)
	window, windowRecords := WindowRecords(records, 20)

	if records[0].Record.ExperienceID != "exp-a" || records[2].Record.ExperienceID != "exp-c" {
		t.Fatalf("records sorted = %#v", records)
	}
	if window.Start != "2026-01-11T00:00:00Z" || window.End != "2026-01-31T00:00:00Z" {
		t.Fatalf("window = %#v", window)
	}
	if len(windowRecords) != 2 || windowRecords[0].Record.ExperienceID != "exp-b" || windowRecords[1].Record.ExperienceID != "exp-c" {
		t.Fatalf("window records = %#v", windowRecords)
	}
}

func TestBuildReportMetricsCopiesSummaryCountsAndDefaultsOutcomeMap(t *testing.T) {
	metrics := BuildReportMetrics(
		RecurrenceSummary{
			HeadlineERR:               0.25,
			ConfirmedRecurrenceCount:  2,
			PossibleRecurrenceCount:   1,
			FlakeDiscountedCount:      3,
			AttributionUncertainCount: 4,
		},
		map[int]bool{11: true, 12: true},
		map[int]bool{12: true},
		5,
		nil,
	)

	if metrics.PRsAnalyzed != 2 || metrics.AgentAttributedPRs != 1 || metrics.AgentAttributedExperiences != 5 {
		t.Fatalf("metric counts = %#v", metrics)
	}
	if metrics.ErrorRecurrenceRate != 0.25 || metrics.ConfirmedRecurrences != 2 || metrics.PossibleRecurrences != 1 {
		t.Fatalf("recurrence metrics = %#v", metrics)
	}
	if metrics.AgentFailuresByOutcomeKind == nil {
		t.Fatal("failure outcome map is nil")
	}
}

func TestBuildReportMetadataIncludesRepoDigestAndFreshness(t *testing.T) {
	records := []Experience{
		{Record: ingestdoc.Record{Metadata: map[string]any{
			"last_ingest_at":               "2026-06-20T00:00:00Z",
			"merged_prs_since_last_ingest": 6,
		}}},
	}
	windowRecords := []Experience{
		{Record: ingestdoc.Record{Repo: ingestdoc.Repo{Owner: "Clyra-AI", Name: "relia"}}},
	}

	metadata := BuildReportMetadata(records, windowRecords, 180, "sha256:source")

	if metadata["repo_id"] != "Clyra-AI/relia" || metadata["window_days"] != 180 || metadata["source_artifact_digest"] != "sha256:source" {
		t.Fatalf("metadata basics = %#v", metadata)
	}
	if metadata["last_ingest_at"] != "2026-06-20T00:00:00Z" || metadata["merged_prs_since_last_ingest"] != 6 {
		t.Fatalf("freshness metadata = %#v", metadata)
	}
	if metadata["network_required"] != false || metadata["repo_relative_paths_only"] != true {
		t.Fatalf("safety metadata = %#v", metadata)
	}
}

func TestBuildReportIDUsesStableWindowDigestAndSummaryInputs(t *testing.T) {
	window := RecurrenceWindow{
		Start: "2026-01-01T00:00:00Z",
		End:   "2026-01-31T00:00:00Z",
	}
	sourceDigest := "sha256:source"
	summary := RecurrenceSummary{
		AgentFailureDenominator:  7,
		ConfirmedRecurrenceCount: 2,
		PossibleRecurrenceCount:  3,
	}
	digestInput := strings.Join([]string{
		window.Start,
		window.End,
		sourceDigest,
		"7",
		"2",
		"3",
	}, "\x00")
	digest := sha256.Sum256([]byte(digestInput))
	expected := "backtest_" + fmt.Sprintf("%x", digest)[:12]

	if id := BuildReportID(window, sourceDigest, summary); id != expected {
		t.Fatalf("report id = %q, want %q", id, expected)
	}

	changed := summary
	changed.PossibleRecurrenceCount = 4
	if BuildReportID(window, sourceDigest, changed) == expected {
		t.Fatal("report id did not change after summary input changed")
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
