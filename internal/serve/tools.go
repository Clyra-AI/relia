package serve

import (
	"path/filepath"
	"sort"
	"strings"

	assessdoc "github.com/Clyra-AI/relia/internal/assess"
	resultdoc "github.com/Clyra-AI/relia/internal/result"
)

const (
	RecallObjectType   = "relia.recall_result"
	CoverageObjectType = "relia.coverage_response"
)

func Manifest(activeRuleCount int, servedRules []map[string]any) map[string]any {
	return map[string]any{
		"transport": "stdio",
		"tools":     []string{"recall", "assess", "coverage"},
		"tool_schemas": []map[string]any{
			{
				"name":        "recall",
				"description": "Return relevant active memory rules with confidence, lifecycle status, and resolved PR citations.",
				"input":       []string{"context", "paths"},
			},
			{
				"name":        "assess",
				"description": "Assess a local unified diff with the same risk engine used by relia assess and advisory planning.",
				"input":       []string{"input"},
			},
			{
				"name":        "coverage",
				"description": "Report active-memory coverage and no_coverage/OOD signals for repo-relative paths.",
				"input":       []string{"paths"},
			},
		},
		"active_rule_count":       activeRuleCount,
		"active_memory_only":      true,
		"citation_required":       true,
		"hosted_service_required": false,
		"live_network_required":   false,
		"served_rules":            servedRules,
	}
}

func BuildRecallResult(root string, options Options, rules []assessdoc.Rule, assessOptions assessdoc.Options) (map[string]any, *resultdoc.CommandError) {
	queryPaths := QueryPaths(options.Context, options.Paths)
	matched := MatchRules(root, rules, queryPaths, options.Context)
	servedRules, commandErr := assessdoc.ServedRuleData(matched, assessOptions)
	if commandErr != nil {
		return nil, commandErr
	}
	coverage := CoverageForRules(matched)
	citations := uniqueStrings(citationsFromRules(matched))
	return map[string]any{
		"object_type":           RecallObjectType,
		"schema_version":        "1.0",
		"query":                 map[string]any{"context": options.Context, "paths": queryPaths},
		"coverage":              coverage,
		"out_of_distribution":   coverage == "no_coverage",
		"rules":                 servedRules,
		"citations":             citations,
		"matched_rule_count":    len(matched),
		"active_rule_count":     len(rules),
		"agent_access_boundary": AgentAccessBoundary(),
		"metadata": map[string]any{
			"active_memory_only":       true,
			"citation_resolution":      citationResolution(citations, matched),
			"repo_relative_paths_only": true,
			"hosted_service_required":  false,
			"live_network_required":    false,
		},
	}, nil
}

func BuildCoverageResult(root string, paths []string, rules []assessdoc.Rule, assessOptions assessdoc.Options) (map[string]any, *resultdoc.CommandError) {
	queryPaths := NormalizeQueryPaths(paths)
	var matchedForQuery []assessdoc.Rule
	seenMatched := map[string]bool{}
	for _, queryPath := range queryPaths {
		matched := MatchRules(root, rules, []string{queryPath}, "")
		if _, commandErr := assessdoc.ServedRuleData(matched, assessOptions); commandErr != nil {
			return nil, commandErr
		}
		for _, rule := range matched {
			key := rule.Path + "\x00" + rule.ID
			if seenMatched[key] {
				continue
			}
			seenMatched[key] = true
			matchedForQuery = append(matchedForQuery, rule)
		}
	}
	entries, summary := assessdoc.CoverageForPaths(root, queryPaths, matchedForQuery)
	return map[string]any{
		"object_type":           CoverageObjectType,
		"schema_version":        "1.0",
		"entries":               entries,
		"summary":               summary,
		"active_rule_count":     len(rules),
		"agent_access_boundary": AgentAccessBoundary(),
		"metadata": map[string]any{
			"coverage_source":          "active_memory_rules",
			"no_coverage_means":        "no active reviewed Relia memory rule currently covers the path",
			"repo_relative_paths_only": true,
		},
	}, nil
}

func AgentAccessBoundary() map[string]any {
	return map[string]any{
		"active_memory_only":       true,
		"served_statuses":          []string{"active"},
		"required_review_label":    "accepted",
		"required_review_gate":     "human_review",
		"required_review_decision": "approved",
		"citation_required":        true,
		"hosted_service_required":  false,
		"live_network_required":    false,
	}
}

func MatchRules(root string, rules []assessdoc.Rule, queryPaths []string, context string) []assessdoc.Rule {
	queryPaths = NormalizeQueryPaths(queryPaths)
	var matched []assessdoc.Rule
	for _, rule := range rules {
		if len(queryPaths) > 0 && assessdoc.RuleMatchesTouchedPath(root, rule, queryPaths) {
			matched = append(matched, rule)
			continue
		}
		if len(queryPaths) == 0 && contextMatchesRule(root, rule, context) {
			matched = append(matched, rule)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Confidence == matched[j].Confidence {
			return matched[i].ID < matched[j].ID
		}
		return matched[i].Confidence > matched[j].Confidence
	})
	return matched
}

func QueryPaths(context string, paths []string) []string {
	queryPaths := NormalizeQueryPaths(paths)
	if len(queryPaths) > 0 {
		return queryPaths
	}
	return ExtractRepoPaths(context)
}

func NormalizeQueryPaths(paths []string) []string {
	var normalized []string
	for _, value := range paths {
		clean, ok := cleanRepoPath(value)
		if ok {
			normalized = append(normalized, filepath.ToSlash(clean))
		}
	}
	return uniqueStrings(normalized)
}

func ExtractRepoPaths(context string) []string {
	replacer := strings.NewReplacer(
		"`", " ",
		"\"", " ",
		"'", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		",", " ",
		";", " ",
		":", " ",
		"\n", " ",
		"\t", " ",
	)
	var paths []string
	for _, field := range strings.Fields(replacer.Replace(context)) {
		field = strings.Trim(field, ".")
		if !strings.Contains(field, "/") {
			continue
		}
		clean, ok := cleanRepoPath(field)
		if ok {
			paths = append(paths, filepath.ToSlash(clean))
		}
	}
	return uniqueStrings(paths)
}

func CoverageForRules(rules []assessdoc.Rule) string {
	if len(rules) == 0 {
		return "no_coverage"
	}
	for _, rule := range rules {
		if rule.Kind == "avoid" {
			return "covered_risky"
		}
	}
	return "covered_clean"
}

func contextMatchesRule(root string, rule assessdoc.Rule, context string) bool {
	lowered := strings.ToLower(context)
	if lowered == "" {
		return false
	}
	for _, rawScope := range rule.ScopePaths {
		scopePath, directoryScope, ok := assessdoc.NormalizeScopePath(root, rawScope)
		if !ok {
			continue
		}
		scopeLower := strings.ToLower(scopePath)
		if strings.Contains(lowered, scopeLower) {
			return true
		}
		if directoryScope {
			prefix := strings.TrimSuffix(scopeLower, "/") + "/"
			for _, queryPath := range ExtractRepoPaths(context) {
				if strings.HasPrefix(strings.ToLower(queryPath), prefix) {
					return true
				}
			}
		}
	}
	return false
}

func citationsFromRules(rules []assessdoc.Rule) []string {
	var citations []string
	for _, rule := range rules {
		citations = append(citations, assessdoc.ServedRuleCitationURLs(rule)...)
	}
	return citations
}

func citationResolution(citations []string, rules []assessdoc.Rule) string {
	if len(rules) == 0 {
		return "not_applicable"
	}
	if len(citations) == 0 {
		return "missing"
	}
	return "resolved"
}

func ruleIDs(rules []assessdoc.Rule) []string {
	var ids []string
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	if ids == nil {
		return []string{}
	}
	return ids
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	sort.Strings(unique)
	if unique == nil {
		return []string{}
	}
	return unique
}

func cleanRepoPath(rel string) (string, bool) {
	trimmed := strings.TrimSpace(rel)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return "", false
	}
	clean := filepath.Clean(trimmed)
	cleanSlash := filepath.ToSlash(clean)
	if clean == "." || clean == ".." || strings.HasPrefix(cleanSlash, "../") {
		return "", false
	}
	for _, part := range strings.Split(cleanSlash, "/") {
		if part == ".." {
			return "", false
		}
	}
	return clean, true
}
