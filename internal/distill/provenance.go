package distill

import (
	"fmt"
	"sort"
	"strings"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

func ReviewLabel(status string, confidence float64) string {
	switch status {
	case "active":
		return "accepted"
	case "stale", "contradicted", "retired":
		return "needs_user_input"
	default:
		if confidence < 0.55 {
			return "needs_user_input"
		}
		return "suggested"
	}
}

func RuleExperienceIDs(records []backtestdoc.Experience) []string {
	var ids []string
	for _, record := range records {
		if strings.TrimSpace(record.Record.ExperienceID) != "" {
			ids = append(ids, record.Record.ExperienceID)
		}
	}
	return uniqueStrings(ids)
}

func RuleProvenanceRefs(records []backtestdoc.Experience) []RuleProvenance {
	seen := map[string]bool{}
	var refs []RuleProvenance
	for _, record := range records {
		url := ingestdoc.PrimaryProvenanceURL(record.Record)
		key := fmt.Sprintf("%d\x00%s\x00%s", record.Record.Action.PR, record.Record.Outcome.Kind, url)
		if seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, RuleProvenance{
			PR:           record.Record.Action.PR,
			Outcome:      record.Record.Outcome.Kind,
			URL:          url,
			ExperienceID: record.Record.ExperienceID,
		})
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].PR == refs[j].PR {
			return refs[i].ExperienceID < refs[j].ExperienceID
		}
		return refs[i].PR < refs[j].PR
	})
	return refs
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
