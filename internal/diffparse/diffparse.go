package diffparse

import (
	"errors"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	ErrNoRepoRelativePaths  = errors.New("diff contains no repo-relative paths")
	unifiedHunkHeaderRegexp = regexp.MustCompile(`^@@ -\d+(?:,(\d+))? \+\d+(?:,(\d+))? @@`)
)

func TouchedPathsOrPlan(content []byte) ([]string, string, error) {
	paths, err := TouchedPaths(content)
	if err == nil {
		return paths, "diff", nil
	}
	if !errors.Is(err, ErrNoRepoRelativePaths) {
		return nil, "", err
	}
	paths, planErr := PlanPaths(content)
	if planErr != nil {
		return nil, "", err
	}
	return paths, "plan", nil
}

func PlanPaths(content []byte) ([]string, error) {
	touched := map[string]bool{}
	quotedFields, planText := quotedPlanFields(string(content))
	for _, field := range quotedFields {
		recordQuotedPlanPath(touched, field)
	}
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
		"<", " ",
		">", " ",
		",", " ",
		";", " ",
		":", " ",
		"\n", " ",
		"\r", " ",
		"\t", " ",
	)
	for _, field := range strings.Fields(replacer.Replace(planText)) {
		recordPlanPath(touched, field)
	}
	paths := make([]string, 0, len(touched))
	for touchedPath := range touched {
		paths = append(paths, touchedPath)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, ErrNoRepoRelativePaths
	}
	return paths, nil
}

func recordQuotedPlanPath(touched map[string]bool, field string) {
	parts := strings.Fields(field)
	if len(parts) <= 1 || quotedFieldLooksLikeSinglePath(parts[0]) {
		recordPlanPath(touched, field)
		return
	}
	for _, part := range parts {
		recordPlanPath(touched, part)
	}
}

func quotedFieldLooksLikeSinglePath(firstField string) bool {
	return strings.Contains(firstField, "/") || strings.HasPrefix(firstField, ".")
}

func quotedPlanFields(raw string) ([]string, string) {
	var fields []string
	var remaining strings.Builder
	for index := 0; index < len(raw); {
		quote := raw[index]
		if quote == '`' && index+2 < len(raw) && raw[index+1] == '`' && raw[index+2] == '`' {
			remaining.WriteString("   ")
			index += 3
			continue
		}
		if quote != '"' && quote != '`' {
			remaining.WriteByte(raw[index])
			index++
			continue
		}
		endOffset := strings.IndexByte(raw[index+1:], quote)
		if endOffset < 0 {
			remaining.WriteByte(raw[index])
			index++
			continue
		}
		fields = append(fields, raw[index+1:index+1+endOffset])
		remaining.WriteByte(' ')
		index += endOffset + 2
	}
	return fields, remaining.String()
}

func recordPlanPath(touched map[string]bool, field string) {
	field = strings.TrimSpace(field)
	field = strings.TrimRight(field, "!?")
	if !strings.HasSuffix(field, "...") {
		field = strings.TrimRight(field, ".")
	}
	if field == "" || strings.Contains(field, "://") {
		return
	}
	if index := strings.Index(field, "#"); index >= 0 {
		field = field[:index]
	}
	field = planPathToken(field)
	if field == "" {
		return
	}
	if !planPathCandidate(field) {
		return
	}
	if cleanPath, ok := normalizedDiffPath(field, false); ok {
		touched[cleanPath] = true
	}
}

func planPathToken(field string) string {
	if !strings.HasPrefix(field, "-") {
		return field
	}
	_, value, found := strings.Cut(field, "=")
	if !found {
		return ""
	}
	return value
}

func planPathCandidate(field string) bool {
	if strings.ContainsAny(field, `\*?`) || strings.Contains(field, "...") {
		return false
	}
	if strings.Contains(field, "/") {
		return slashPathCandidate(field)
	}
	switch field {
	case "Makefile", "Dockerfile", "LICENSE", "NOTICE", "README", "CHANGELOG":
		return true
	}
	if abbreviationLikeToken(field) {
		return false
	}
	extension := filepath.Ext(field)
	return extension != "" && hasASCIIAlpha(extension)
}

func slashPathCandidate(value string) bool {
	if remotePathLikeToken(value) {
		return false
	}
	if slashProseToken(value) {
		return false
	}
	parts := strings.Split(value, "/")
	return len(parts) >= 2 && parts[0] != ""
}

func slashProseToken(value string) bool {
	switch strings.ToLower(value) {
	case "and/or", "ci/cd", "read/write", "write/read", "input/output", "output/input",
		"inputs/outputs", "outputs/inputs", "on/off", "yes/no", "before/after",
		"after/before", "client/server", "server/client", "frontend/backend",
		"backend/frontend", "producer/consumer", "consumer/producer", "request/response",
		"response/request":
		return true
	default:
		return false
	}
}

func remotePathLikeToken(value string) bool {
	first, _, found := strings.Cut(value, "/")
	if !found || strings.HasPrefix(first, ".") {
		return false
	}
	return strings.Contains(first, ".")
}

func abbreviationLikeToken(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if len(part) != 1 || !isASCIIAlpha(rune(part[0])) {
			return false
		}
	}
	return true
}

func hasASCIIAlpha(value string) bool {
	for _, char := range value {
		if isASCIIAlpha(char) {
			return true
		}
	}
	return false
}

func isASCIIAlpha(char rune) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

func TouchedPaths(content []byte) ([]string, error) {
	touched := map[string]bool{}
	inFileHeader := false
	gitFileHeader := false
	currentHeaderAdded := map[string]bool{}
	currentMetadataPaths := []string{}
	hunkOldRemaining := 0
	hunkNewRemaining := 0
	lines := strings.Split(string(content), "\n")
	for index, line := range lines {
		line = strings.TrimRight(line, "\r")
		if hunkOldRemaining > 0 || hunkNewRemaining > 0 {
			hunkOldRemaining, hunkNewRemaining = consumeUnifiedHunkLine(line, hunkOldRemaining, hunkNewRemaining)
			continue
		}
		switch {
		case strings.HasPrefix(line, "diff --git "):
			reconcileSyntheticHeaderPaths(touched, currentHeaderAdded, currentMetadataPaths)
			inFileHeader = true
			currentHeaderAdded = map[string]bool{}
			currentMetadataPaths = nil
			headerPaths, stripGitHeaderPrefix := diffGitHeaderPaths(line)
			gitFileHeader = stripGitHeaderPrefix
			for _, path := range headerPaths {
				if cleanPath, ok := normalizedDiffPath(path, stripGitHeaderPrefix); ok {
					_, existed := touched[cleanPath]
					touched[cleanPath] = true
					if _, tracked := currentHeaderAdded[cleanPath]; !tracked {
						currentHeaderAdded[cleanPath] = !existed
					}
				}
			}
		case inFileHeader && strings.HasPrefix(line, "--- "):
			if !gitFileHeader {
				gitFileHeader = gitStyleUnifiedFileHeader(lines, index)
			}
			addDiffPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "--- ")), gitFileHeader)
		case strings.HasPrefix(line, "--- ") && plainUnifiedFileHeader(lines, index):
			inFileHeader = true
			gitFileHeader = false
			addDiffPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "--- ")), false)
		case inFileHeader && strings.HasPrefix(line, "+++ "):
			addDiffPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "+++ ")), gitFileHeader)
		case inFileHeader && strings.HasPrefix(line, "rename from "):
			currentMetadataPaths = append(currentMetadataPaths, addDiffMetadataPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "rename from "))))
		case inFileHeader && strings.HasPrefix(line, "rename to "):
			currentMetadataPaths = append(currentMetadataPaths, addDiffMetadataPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "rename to "))))
		case inFileHeader && strings.HasPrefix(line, "copy from "):
			currentMetadataPaths = append(currentMetadataPaths, addDiffMetadataPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "copy from "))))
		case inFileHeader && strings.HasPrefix(line, "copy to "):
			currentMetadataPaths = append(currentMetadataPaths, addDiffMetadataPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "copy to "))))
		case strings.HasPrefix(line, "@@"):
			reconcileSyntheticHeaderPaths(touched, currentHeaderAdded, currentMetadataPaths)
			inFileHeader = false
			gitFileHeader = false
			currentMetadataPaths = nil
			hunkOldRemaining, hunkNewRemaining = parseUnifiedHunkCounts(line)
		}
	}
	reconcileSyntheticHeaderPaths(touched, currentHeaderAdded, currentMetadataPaths)
	paths := make([]string, 0, len(touched))
	for touchedPath := range touched {
		paths = append(paths, touchedPath)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, ErrNoRepoRelativePaths
	}
	return paths, nil
}

func plainUnifiedFileHeader(lines []string, index int) bool {
	if index+2 >= len(lines) {
		return false
	}
	return strings.HasPrefix(strings.TrimRight(lines[index+1], "\r"), "+++ ") &&
		strings.HasPrefix(strings.TrimRight(lines[index+2], "\r"), "@@")
}

func gitStyleUnifiedFileHeader(lines []string, index int) bool {
	if index+1 >= len(lines) {
		return false
	}
	oldPath := strings.TrimSpace(strings.TrimPrefix(strings.TrimRight(lines[index], "\r"), "--- "))
	newLine := strings.TrimRight(lines[index+1], "\r")
	if !strings.HasPrefix(newLine, "+++ ") {
		return false
	}
	newPath := strings.TrimSpace(strings.TrimPrefix(newLine, "+++ "))
	return strings.HasPrefix(oldPath, "a/") && strings.HasPrefix(newPath, "b/")
}

func parseUnifiedHunkCounts(line string) (int, int) {
	matches := unifiedHunkHeaderRegexp.FindStringSubmatch(line)
	if len(matches) == 0 {
		return 0, 0
	}
	oldCount := 1
	newCount := 1
	if matches[1] != "" {
		if parsed, err := strconv.Atoi(matches[1]); err == nil {
			oldCount = parsed
		}
	}
	if matches[2] != "" {
		if parsed, err := strconv.Atoi(matches[2]); err == nil {
			newCount = parsed
		}
	}
	return oldCount, newCount
}

func consumeUnifiedHunkLine(line string, oldRemaining int, newRemaining int) (int, int) {
	if line == "" {
		return oldRemaining, newRemaining
	}
	switch line[0] {
	case ' ':
		oldRemaining--
		newRemaining--
	case '-':
		oldRemaining--
	case '+':
		newRemaining--
	}
	if oldRemaining < 0 {
		oldRemaining = 0
	}
	if newRemaining < 0 {
		newRemaining = 0
	}
	return oldRemaining, newRemaining
}

func diffGitHeaderPaths(line string) ([]string, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "diff --git "))
	if strings.HasPrefix(rest, "a/") {
		if paths := prefixedGitDiffHeaderPaths(rest); len(paths) > 0 {
			return paths, true
		}
		if paths := identicalNoPrefixDiffHeaderPaths(rest); len(paths) > 0 {
			return paths, false
		}
		return nil, false
	}
	if !strings.HasPrefix(rest, "\"") {
		return identicalNoPrefixDiffHeaderPaths(rest), false
	}
	var paths []string
	for len(rest) > 0 && len(paths) < 2 {
		var path string
		var ok bool
		path, rest, ok = nextDiffHeaderPath(rest)
		if !ok {
			break
		}
		paths = append(paths, path)
	}
	if len(paths) == 2 && quotedOrRawPathHasPrefix(paths[0], "a/") && quotedOrRawPathHasPrefix(paths[1], "b/") {
		return paths, true
	}
	return paths, false
}

func prefixedGitDiffHeaderPaths(rest string) []string {
	type candidate struct {
		left  string
		right string
	}
	var candidates []candidate
	searchStart := 0
	for {
		index := strings.Index(rest[searchStart:], " b/")
		if index < 0 {
			break
		}
		split := searchStart + index
		left := rest[:split]
		right := rest[split+1:]
		if strings.HasPrefix(left, "a/") && strings.HasPrefix(right, "b/") {
			if strings.TrimPrefix(left, "a/") == strings.TrimPrefix(right, "b/") {
				return []string{left, right}
			}
			candidates = append(candidates, candidate{left: left, right: right})
		}
		searchStart = split + len(" b/")
		if searchStart >= len(rest) {
			break
		}
	}
	if len(candidates) == 1 {
		return []string{candidates[0].left, candidates[0].right}
	}
	return nil
}

func identicalNoPrefixDiffHeaderPaths(rest string) []string {
	fields := strings.Fields(rest)
	for split := 1; split < len(fields); split++ {
		left := strings.Join(fields[:split], " ")
		right := strings.Join(fields[split:], " ")
		if left != "" && left == right {
			return []string{left, right}
		}
	}
	return nil
}

func quotedOrRawPathHasPrefix(path string, prefix string) bool {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "\"") {
		if unquoted, err := strconv.Unquote(path); err == nil {
			path = unquoted
		}
	}
	return strings.HasPrefix(path, prefix)
}

func nextDiffHeaderPath(input string) (string, string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", false
	}
	if strings.HasPrefix(input, "\"") {
		escaped := false
		for index := 1; index < len(input); index++ {
			char := input[index]
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				token := input[:index+1]
				if _, err := strconv.Unquote(token); err == nil {
					return token, input[index+1:], true
				}
				return strings.Trim(token, "\""), input[index+1:], true
			}
		}
	}
	if index := strings.IndexAny(input, "\t "); index >= 0 {
		return input[:index], input[index+1:], true
	}
	return input, "", true
}

func addDiffPath(touched map[string]bool, raw string, stripGitPrefix bool) {
	if cleanPath, ok := normalizedDiffPath(raw, stripGitPrefix); ok {
		touched[cleanPath] = true
	}
}

func normalizedDiffPath(raw string, stripGitPrefix bool) (string, bool) {
	pathPart := strings.TrimSpace(raw)
	quoted := strings.HasPrefix(pathPart, "\"")
	if quoted {
		if unquoted, err := strconv.Unquote(pathPart); err == nil {
			pathPart = unquoted
		}
	}
	if !quoted {
		if index := strings.Index(pathPart, "\t"); index >= 0 {
			pathPart = pathPart[:index]
		}
	}
	if pathPart == "" || pathPart == "/dev/null" {
		return "", false
	}
	if stripGitPrefix && (strings.HasPrefix(pathPart, "a/") || strings.HasPrefix(pathPart, "b/")) {
		pathPart = pathPart[2:]
	}
	if clean, ok := cleanRepoPath(pathPart); ok {
		return filepath.ToSlash(clean), true
	}
	return "", false
}

func addDiffMetadataPath(touched map[string]bool, raw string) string {
	addDiffPath(touched, raw, false)
	return raw
}

func reconcileSyntheticHeaderPaths(touched map[string]bool, currentHeaderAdded map[string]bool, metadataPaths []string) {
	for leftIndex, left := range metadataPaths {
		for _, right := range metadataPaths[leftIndex+1:] {
			if stripped, ok := matchingSyntheticMetadataPath(left, right); ok && currentHeaderAdded[stripped] {
				delete(touched, stripped)
			}
		}
	}
}

func matchingSyntheticMetadataPath(left string, right string) (string, bool) {
	leftPath, leftOK := normalizedDiffPath(left, false)
	rightPath, rightOK := normalizedDiffPath(right, false)
	if !leftOK || !rightOK {
		return "", false
	}
	if strings.HasPrefix(leftPath, "a/") && strings.HasPrefix(rightPath, "b/") && strings.TrimPrefix(leftPath, "a/") == strings.TrimPrefix(rightPath, "b/") {
		return strings.TrimPrefix(leftPath, "a/"), true
	}
	if strings.HasPrefix(leftPath, "b/") && strings.HasPrefix(rightPath, "a/") && strings.TrimPrefix(leftPath, "b/") == strings.TrimPrefix(rightPath, "a/") {
		return strings.TrimPrefix(leftPath, "b/"), true
	}
	return "", false
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
