package distill

import (
	"path/filepath"
	"strings"
)

func StatusCounts(rules []Rule) map[string]int {
	counts := map[string]int{}
	for _, rule := range rules {
		counts[rule.Status]++
	}
	return counts
}

func DraftedRuleData(rules []Rule, artifactPaths []string) []map[string]any {
	pathsByID := map[string]string{}
	for _, artifactPath := range artifactPaths {
		id := strings.TrimSuffix(filepath.Base(artifactPath), filepath.Ext(artifactPath))
		pathsByID[id] = artifactPath
	}
	data := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		data = append(data, map[string]any{
			"id":               rule.ID,
			"kind":             rule.Kind,
			"status":           rule.Status,
			"review_label":     rule.ReviewLabel,
			"confidence":       rule.Confidence,
			"confidence_label": rule.Metadata.ConfidenceLabel,
			"path":             pathsByID[rule.ID],
		})
	}
	return data
}
