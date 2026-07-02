package backtest

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	configdoc "github.com/Clyra-AI/relia/internal/config"
)

const automaticFlakeDiscountReason = "Discounted as flaky because the same failure signature appears at least three times across unrelated non-test diff paths."

func AutomaticFlakeDiscounts(records []Experience) map[string]string {
	bySignature := map[string][]Experience{}
	for _, record := range records {
		if record.Record.Attribution.ActorKind != "agent" || !IsFailureOutcome(record.Record.Outcome.Kind) {
			continue
		}
		for _, key := range RecurrenceSignatureKeys(record.Record) {
			bySignature[key] = append(bySignature[key], record)
		}
	}
	discounted := map[string]string{}
	for _, group := range bySignature {
		if len(group) < 3 || !groupHasUnrelatedNonTestDiffs(group) {
			continue
		}
		for _, record := range group {
			if record.Record.FlakeDiscount == 0 && discounted[record.Record.ExperienceID] == "" {
				discounted[record.Record.ExperienceID] = automaticFlakeDiscountReason
			}
		}
	}
	return discounted
}

func IsFlakeDiscounted(record Experience, heuristics map[string]string) bool {
	return record.Record.FlakeDiscount > 0 || heuristics[record.Record.ExperienceID] != ""
}

func BuildFlakeDiscount(record Experience, records []Experience, heuristics map[string]string) FlakeDiscount {
	supportingPRs := []int{}
	supportingRefs := []string{}
	for _, candidate := range records {
		if candidate.Record.ExperienceID == record.Record.ExperienceID {
			continue
		}
		if !RecordsShareRecurrenceSignature(candidate.Record, record.Record) {
			continue
		}
		if candidate.Record.Attribution.ActorKind != "agent" || !IsFailureOutcome(candidate.Record.Outcome.Kind) {
			continue
		}
		supportingPRs = append(supportingPRs, candidate.Record.Action.PR)
		supportingRefs = append(supportingRefs, SourceLineRef(candidate))
	}
	sort.Ints(supportingPRs)
	supportingRefs = uniqueStrings(supportingRefs)
	reason := heuristics[record.Record.ExperienceID]
	flakeDiscount := record.Record.FlakeDiscount
	if reason == "" {
		reason = "Discounted as flaky because the experience record carries an explicit flake_discount."
	}
	if flakeDiscount == 0 {
		flakeDiscount = 1
	}
	return FlakeDiscount{
		ExperienceID:    record.Record.ExperienceID,
		PR:              record.Record.Action.PR,
		SignatureID:     record.Record.Outcome.Signature.SignatureID,
		FlakeDiscount:   roundFloat(flakeDiscount, 4),
		SupportingPRs:   supportingPRs,
		SupportingRefs:  supportingRefs,
		Reason:          reason,
		ExcludedFromERR: true,
	}
}

func groupHasUnrelatedNonTestDiffs(group []Experience) bool {
	seen := map[string]bool{}
	for _, record := range group {
		paths := nonTestPaths(record.Record.Context.Paths)
		if len(paths) == 0 {
			paths = normalizedRepoPaths(record.Record.Context.Paths)
		}
		for _, path := range paths {
			if seen[path] {
				return false
			}
			seen[path] = true
		}
	}
	return len(seen) >= len(group)
}

func nonTestPaths(paths []string) []string {
	var result []string
	for _, clean := range normalizedRepoPaths(paths) {
		base := path.Base(clean)
		if strings.HasPrefix(clean, "tests/") || strings.Contains(clean, "/tests/") || strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.go") {
			continue
		}
		result = append(result, clean)
	}
	return result
}

func normalizedRepoPaths(paths []string) []string {
	var result []string
	for _, value := range paths {
		if clean, ok := configdoc.CleanRepoPath(value); ok {
			result = append(result, filepath.ToSlash(clean))
		}
	}
	result = uniqueStrings(result)
	sort.Strings(result)
	return result
}
