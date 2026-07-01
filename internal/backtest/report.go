package backtest

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	badgeStaleAfterDays      = 30
	badgeStaleAfterMergedPRs = 20
	badgeMetadataLastIngest  = "last_ingest_at"
	badgeMetadataMergedPRs   = "merged_prs_since_last_ingest"
)

func BuildTopRepeatedMistakes(pairs []RecurrencePair) []TopRepeatedMistake {
	type aggregate struct {
		signatureID   string
		repeatCount   int
		prs           []int
		urls          []string
		experienceIDs []string
		refs          []string
	}
	bySignature := map[string]*aggregate{}
	for _, pair := range pairs {
		key := pair.SignatureID
		if pair.MatchedSignatureID != "" {
			key = pair.MatchedSignatureID
		}
		if key == "" {
			continue
		}
		item := bySignature[key]
		if item == nil {
			item = &aggregate{signatureID: key}
			bySignature[key] = item
		}
		item.repeatCount++
		item.prs = append(item.prs, pair.PriorPR, pair.CurrentPR)
		item.urls = append(item.urls, pair.PriorURL, pair.CurrentURL)
		item.experienceIDs = append(item.experienceIDs, pair.PriorExperienceID, pair.CurrentExperienceID)
		item.refs = append(item.refs, pair.Refs...)
	}
	aggregates := make([]*aggregate, 0, len(bySignature))
	for _, item := range bySignature {
		item.prs = uniqueInts(item.prs)
		item.urls = uniqueStrings(item.urls)
		item.experienceIDs = uniqueStrings(item.experienceIDs)
		item.refs = uniqueStrings(item.refs)
		sort.Ints(item.prs)
		sort.Strings(item.urls)
		sort.Strings(item.experienceIDs)
		sort.Strings(item.refs)
		aggregates = append(aggregates, item)
	}
	sort.Slice(aggregates, func(i, j int) bool {
		if aggregates[i].repeatCount == aggregates[j].repeatCount {
			return aggregates[i].signatureID < aggregates[j].signatureID
		}
		return aggregates[i].repeatCount > aggregates[j].repeatCount
	})
	result := make([]TopRepeatedMistake, 0, len(aggregates))
	for index, item := range aggregates {
		result = append(result, TopRepeatedMistake{
			Rank:          index + 1,
			SignatureID:   item.signatureID,
			RepeatCount:   item.repeatCount,
			PRs:           item.prs,
			URLs:          item.urls,
			ExperienceIDs: item.experienceIDs,
			Refs:          item.refs,
		})
	}
	return result
}

func BuildReportDiagnostics(summary RecurrenceSummary, baseline BaselineComparison, sourceArtifacts []string) []ReportDiagnostic {
	ref := "schemas/experience-record.schema.json"
	if len(sourceArtifacts) > 0 {
		ref = sourceArtifacts[0]
	}
	diagnostics := []ReportDiagnostic{
		{
			Type:    "memory_source_verified",
			Status:  "pass",
			Message: "Backtest, distill, and memory outputs are derived from canonical experience records; agent self-reports and reflections are rejected before persistence.",
			Ref:     ref,
		},
	}
	if summary.PossibleRecurrenceCount > 0 {
		diagnostics = append(diagnostics, ReportDiagnostic{
			Type:    "possible_recurrences_excluded",
			Status:  "info",
			Message: "Possible recurrences are reported separately and excluded from headline ERR.",
			Ref:     "schemas/recurrence-report.schema.json",
		})
	}
	if summary.FlakeDiscountedCount > 0 {
		diagnostics = append(diagnostics, ReportDiagnostic{
			Type:    "flake_discounts_visible",
			Status:  "info",
			Message: "Flake-discounted failures remain visible and are excluded from the recurrence numerator.",
			Ref:     "schemas/recurrence-report.schema.json",
		})
	}
	if summary.AttributionUncertainCount > 0 {
		diagnostics = append(diagnostics, ReportDiagnostic{
			Type:    "uncertain_attribution_excluded",
			Status:  "info",
			Message: "Uncertain attribution is excluded from headline ERR by default.",
			Ref:     "relia.yaml",
		})
	}
	if baseline.Stale {
		diagnostics = append(diagnostics, ReportDiagnostic{
			Type:    "stale_baseline",
			Status:  "warn",
			Message: baseline.Reason,
			Ref:     baseline.Path,
		})
	}
	return diagnostics
}

func BuildReportOperatorFeedback(summary RecurrenceSummary) OperatorFeedback {
	nextCommand := "relia ingest --input <outcomes.jsonl>"
	if summary.AgentFailureDenominator > 0 {
		nextCommand = "relia distill --format json"
	}
	return OperatorFeedback{
		Summary: fmt.Sprintf("%s ERR from %d confirmed recurrences across %d agent-attributed failures.",
			summary.HeadlineERRPercent,
			summary.ConfirmedRecurrenceCount,
			summary.AgentFailureDenominator),
		ConservativeMatchingNote: "Headline ERR counts confirmed recurrences only; possible recurrences, flake discounts, and uncertain attribution are visible but excluded from the headline.",
		NextCommand:              nextCommand,
	}
}

func ReportRepoID(records []Experience) string {
	if len(records) == 0 {
		return ""
	}
	repo := records[0].Record.Repo
	if repo.Owner == "" || repo.Name == "" {
		return ""
	}
	return repo.Owner + "/" + repo.Name
}

func ReportIngestFreshnessMetadata(records []Experience) (time.Time, int, bool, bool) {
	var latestIngestAt time.Time
	mergedSinceIngest := 0
	hasLastIngestAt := false
	hasMergedSinceIngest := false
	for _, record := range records {
		ingestedAt, ok := metadataTime(record.Record.Metadata, badgeMetadataLastIngest)
		if !ok {
			continue
		}
		if !hasLastIngestAt || ingestedAt.After(latestIngestAt) {
			latestIngestAt = ingestedAt
			mergedSinceIngest = 0
			hasLastIngestAt = true
			hasMergedSinceIngest = false
		}
		if !ingestedAt.Equal(latestIngestAt) {
			continue
		}
		if value, ok := metadataInt(record.Record.Metadata, badgeMetadataMergedPRs); ok {
			if !hasMergedSinceIngest || value > mergedSinceIngest {
				mergedSinceIngest = value
			}
			hasMergedSinceIngest = true
		}
	}
	return latestIngestAt, mergedSinceIngest, hasLastIngestAt, hasMergedSinceIngest
}

func BuildReportBadge(report RecurrenceReport) ReportBadge {
	return BuildReportBadgeAt(report, time.Now().UTC())
}

func BuildReportBadgeAt(report RecurrenceReport, now time.Time) ReportBadge {
	color := "brightgreen"
	switch {
	case report.HeadlineERR >= 0.3:
		color = "orange"
	case report.HeadlineERR >= 0.1:
		color = "yellow"
	}

	status := "current"
	message := "ERR " + report.Summary.HeadlineERRPercent
	stale, reason := reportBadgeStaleness(report, now)
	if stale {
		status = "stale"
		message += " stale"
		color = "lightgrey"
	}

	return ReportBadge{
		Label:          "Relia",
		Message:        message,
		Status:         status,
		Stale:          stale,
		Color:          color,
		Reason:         reason,
		SourceReportID: report.ReportID,
	}
}

func reportBadgeStaleness(report RecurrenceReport, now time.Time) (bool, string) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	lastIngestAt, hasLastIngestAt := metadataTime(report.Metadata, badgeMetadataLastIngest)
	if !hasLastIngestAt {
		return true, "Ingest freshness is unavailable; rerun relia ingest before publishing the README badge."
	}
	if now.UTC().Sub(lastIngestAt.UTC()) > time.Duration(badgeStaleAfterDays)*24*time.Hour {
		return true, fmt.Sprintf("Last ingest exceeds the %d-day freshness window; rerun relia ingest and backtest before publishing the README badge.", badgeStaleAfterDays)
	}
	mergedSinceIngest, hasMergedSinceIngest := metadataInt(report.Metadata, badgeMetadataMergedPRs)
	if !hasMergedSinceIngest {
		return true, "Merged PR activity freshness is unavailable; provide merged_prs_since_last_ingest metadata before publishing the README badge."
	}
	if mergedSinceIngest > badgeStaleAfterMergedPRs {
		return true, fmt.Sprintf("More than %d PRs have merged since the last ingest; rerun relia ingest and backtest before publishing the README badge.", badgeStaleAfterMergedPRs)
	}
	return false, fmt.Sprintf("Generated from ingest metadata within the %d-day freshness window and %d merged PRs since ingest.", badgeStaleAfterDays, mergedSinceIngest)
}

func metadataInt(metadata map[string]any, key string) (int, bool) {
	value, ok := metadata[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		if typed < 0 {
			return 0, false
		}
		return typed, true
	case int64:
		if typed < 0 {
			return 0, false
		}
		return int64ToInt(typed)
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed < 0 || typed != math.Trunc(typed) {
			return 0, false
		}
		return int64ToInt(int64(typed))
	case json.Number:
		number, err := typed.Int64()
		if err != nil || number < 0 {
			return 0, false
		}
		return int64ToInt(number)
	default:
		return 0, false
	}
}

func metadataTime(metadata map[string]any, key string) (time.Time, bool) {
	value, ok := metadata[key]
	if !ok {
		return time.Time{}, false
	}
	text := stringFromAny(value)
	if text == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueInts(values []int) []int {
	seen := map[int]struct{}{}
	var result []int
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func int64ToInt(value int64) (int, bool) {
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	if value < minInt || value > maxInt {
		return 0, false
	}
	return int(value), true
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}
