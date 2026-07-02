package distill

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
)

func ScopePaths(records []backtestdoc.Experience) []string {
	counts := map[string]int{}
	for _, record := range records {
		paths := NonTestPaths(record.Record.Context.Paths)
		if len(paths) == 0 {
			paths = NormalizedRepoPaths(record.Record.Context.Paths)
		}
		for _, path := range paths {
			counts[path]++
		}
	}
	return topCountedStrings(counts, 3)
}

func ScopeSignals(cluster Cluster, records []backtestdoc.Experience) []string {
	counts := map[string]int{}
	if cluster.Signal != "" {
		counts[cluster.Signal]++
	}
	for _, record := range records {
		signal := RecordSignal(record.Record)
		if signal != "" {
			counts[signal]++
		}
	}
	return topCountedStrings(counts, 3)
}

func NonTestPaths(paths []string) []string {
	var result []string
	for _, clean := range NormalizedRepoPaths(paths) {
		base := path.Base(clean)
		if strings.HasPrefix(clean, "tests/") || strings.Contains(clean, "/tests/") || strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.go") {
			continue
		}
		result = append(result, clean)
	}
	return result
}

func NormalizedRepoPaths(paths []string) []string {
	var result []string
	for _, value := range paths {
		if clean, ok := cleanRepoPath(value); ok {
			result = append(result, filepath.ToSlash(clean))
		}
	}
	result = uniqueStrings(result)
	sort.Strings(result)
	return result
}

func topCountedStrings(counts map[string]int, limit int) []string {
	type counted struct {
		Value string
		Count int
	}
	var values []counted
	for value, count := range counts {
		if strings.TrimSpace(value) == "" {
			continue
		}
		values = append(values, counted{Value: value, Count: count})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Count == values[j].Count {
			return values[i].Value < values[j].Value
		}
		return values[i].Count > values[j].Count
	})
	if limit > 0 && len(values) > limit {
		values = values[:limit]
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Value)
	}
	return result
}
