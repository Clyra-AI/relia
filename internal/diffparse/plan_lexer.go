package diffparse

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type PlanTokenClass string

const (
	PlanTokenQuotedRepoPath        PlanTokenClass = "quoted_repo_path"
	PlanTokenUnquotedRepoPath      PlanTokenClass = "unquoted_repo_path"
	PlanTokenCommandSpan           PlanTokenClass = "command_span"
	PlanTokenCommandHelper         PlanTokenClass = "command_executable_or_helper"
	PlanTokenOption                PlanTokenClass = "option"
	PlanTokenOptionValue           PlanTokenClass = "option_value"
	PlanTokenEnvironmentAssignment PlanTokenClass = "environment_assignment"
	PlanTokenRemote                PlanTokenClass = "remote_url_module_or_domain"
	PlanTokenTaskID                PlanTokenClass = "task_or_acceptance_id"
	PlanTokenProse                 PlanTokenClass = "prose_or_unavailable"
	PlanTokenStructuredPath        PlanTokenClass = "structured_path_field"
	PlanTokenStructuredProse       PlanTokenClass = "structured_prose_field"
	PlanTokenStructuredIgnored     PlanTokenClass = "ignored_structured_field"
)

// PlanToken retains the original byte span and command context used to decide
// whether a plan fragment contributes a repo-relative implementation path.
type PlanToken struct {
	Text     string
	Start    int
	End      int
	Class    PlanTokenClass
	Children []PlanToken
}

type planField struct {
	text  string
	start int
	end   int
}

func recordPlanText(touched map[string]bool, text string) {
	for _, token := range LexPlanText(text) {
		recordPlanToken(touched, token)
	}
}

func recordPlanToken(touched map[string]bool, token PlanToken) {
	switch token.Class {
	case PlanTokenQuotedRepoPath, PlanTokenUnquotedRepoPath, PlanTokenOptionValue, PlanTokenStructuredPath:
		recordPlanPath(touched, token.Text)
	case PlanTokenCommandSpan, PlanTokenStructuredProse:
		for _, child := range token.Children {
			recordPlanToken(touched, child)
		}
	}
}

func LexPlanText(raw string) []PlanToken {
	quoted, remainder := quotedPlanFields(raw)
	tokens := make([]PlanToken, 0, len(quoted)+16)
	for _, field := range quoted {
		tokens = append(tokens, classifyQuotedPlanField(field))
	}
	tokens = append(tokens, classifyUnquotedPlanFields(scanPlanFields(remainder), 0)...)
	sort.SliceStable(tokens, func(i, j int) bool {
		return tokens[i].Start < tokens[j].Start
	})
	return tokens
}

func classifyQuotedPlanField(field planField) PlanToken {
	parts := scanPlanFields(field.text)
	if quotedFieldLooksLikePathWithSpaces(parts) {
		return PlanToken{Text: field.text, Start: field.start, End: field.end, Class: PlanTokenQuotedRepoPath}
	}
	if quotedFieldLooksLikeCommandSpan(parts) {
		children := classifyUnquotedPlanFields(parts, field.start)
		if len(children) > 0 {
			children[0].Class = PlanTokenCommandHelper
			if interpreterCommandToken(commandContextToken(children[0].Text)) {
				for index := 1; index < len(children); index++ {
					if children[index].Class == PlanTokenOption {
						continue
					}
					if commandScriptOperand(children[index].Text) {
						children[index].Class = PlanTokenCommandHelper
					}
					break
				}
			}
		}
		return PlanToken{Text: field.text, Start: field.start, End: field.end, Class: PlanTokenCommandSpan, Children: children}
	}
	token := classifyPlanAtom(field.text, field.start, field.end)
	if token.Class == PlanTokenUnquotedRepoPath {
		token.Class = PlanTokenQuotedRepoPath
	}
	return token
}

func classifyUnquotedPlanFields(fields []planField, base int) []PlanToken {
	tokens := make([]PlanToken, 0, len(fields))
	skipInterpreterScriptOperand := false
	previousMeaningful := ""
	for _, field := range fields {
		token := classifyPlanAtom(field.text, base+field.start, base+field.end)
		normalized := commandContextToken(field.text)
		if skipInterpreterScriptOperand && token.Class != PlanTokenOption {
			skipInterpreterScriptOperand = false
			if commandScriptOperand(field.text) {
				token.Class = PlanTokenCommandHelper
			}
		}
		if unquotedHelperExecutable(field.text) && commandLeadInToken(previousMeaningful) {
			token.Class = PlanTokenCommandHelper
		}
		if knownCommandToken(normalized) && commandLeadInToken(previousMeaningful) {
			token.Class = PlanTokenCommandHelper
		}
		if interpreterCommandToken(normalized) {
			skipInterpreterScriptOperand = true
			token.Class = PlanTokenCommandHelper
		}
		tokens = append(tokens, token)
		if normalized != "" {
			previousMeaningful = normalized
		}
	}
	return tokens
}

func classifyPlanAtom(raw string, start int, end int) PlanToken {
	text := strings.TrimSpace(raw)
	token := PlanToken{Text: text, Start: start, End: end, Class: PlanTokenProse}
	if envAssignmentToken(text) {
		token.Class = PlanTokenEnvironmentAssignment
		return token
	}
	if strings.HasPrefix(text, "-") {
		_, value, found := strings.Cut(text, "=")
		if !found {
			token.Class = PlanTokenOption
			return token
		}
		token.Text = value
		token.Class = PlanTokenOption
		if planPathCandidate(stripLineSuffix(strings.TrimRight(value, ".:!?"))) {
			token.Class = PlanTokenOptionValue
		}
		return token
	}
	cleaned := strings.TrimRight(text, "!?")
	if !strings.HasSuffix(cleaned, "...") {
		cleaned = strings.TrimRight(cleaned, ".")
	}
	cleaned = strings.TrimRight(cleaned, ":")
	if strings.Contains(cleaned, "://") || githubShorthandRef(cleaned) || bareDomainLikeToken(cleaned) || remotePathLikeToken(cleaned) || strings.HasPrefix(cleaned, "git@") {
		token.Class = PlanTokenRemote
		return token
	}
	withoutFragment, _, _ := strings.Cut(cleaned, "#")
	withoutLine := stripLineSuffix(withoutFragment)
	if taskIDLikeSegment(withoutLine) || slashTaskIDChainToken(withoutLine) {
		token.Class = PlanTokenTaskID
		return token
	}
	if planPathCandidate(withoutLine) {
		token.Class = PlanTokenUnquotedRepoPath
	}
	return token
}

func scanPlanFields(raw string) []planField {
	var fields []planField
	start := -1
	for index, char := range raw {
		if unicode.IsSpace(char) || strings.ContainsRune("`()[]{}<>,;\"'", char) {
			if start >= 0 {
				fields = append(fields, planField{text: raw[start:index], start: start, end: index})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = index
		}
	}
	if start >= 0 {
		fields = append(fields, planField{text: raw[start:], start: start, end: len(raw)})
	}
	return fields
}

func quotedPlanFields(raw string) ([]planField, string) {
	fields := []planField{}
	remainder := []byte(raw)
	for index := 0; index < len(raw); {
		quote, quoted := planQuoteAt(raw, index)
		if quote == '`' && index+2 < len(raw) && raw[index+1] == '`' && raw[index+2] == '`' {
			index += 3
			continue
		}
		if !quoted {
			index++
			continue
		}
		endIndex := closingPlanQuoteIndex(raw, index+1, quote)
		if endIndex < 0 {
			index++
			continue
		}
		fields = append(fields, planField{text: raw[index+1 : endIndex], start: index + 1, end: endIndex})
		for blank := index; blank <= endIndex; blank++ {
			remainder[blank] = ' '
		}
		index = endIndex + 1
	}
	return fields, string(remainder)
}

func structuredPlanPaths(content []byte) ([]string, bool) {
	var payload any
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, false
	}
	touched := map[string]bool{}
	recordStructuredPlanPaths(touched, payload, "")
	paths := make([]string, 0, len(touched))
	for touchedPath := range touched {
		paths = append(paths, touchedPath)
	}
	sort.Strings(paths)
	return paths, true
}

func recordStructuredPlanPaths(touched map[string]bool, value any, key string) {
	normalizedKey := strings.ToLower(key)
	if structuredPlanIgnoreKey(normalizedKey) {
		return
	}
	pathKey := structuredPlanPathKey(normalizedKey)
	proseKey := structuredPlanProseKey(normalizedKey)
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			recordStructuredPlanPaths(touched, childValue, childKey)
		}
	case []any:
		for _, childValue := range typed {
			recordStructuredPlanPaths(touched, childValue, normalizedKey)
		}
	case string:
		if proseKey {
			token := PlanToken{Text: typed, Class: PlanTokenStructuredProse, Children: LexPlanText(typed)}
			recordPlanToken(touched, token)
		} else if pathKey {
			recordPlanToken(touched, PlanToken{Text: typed, Class: PlanTokenStructuredPath})
		}
	}
}

func structuredPlanIgnoreKey(key string) bool {
	if strings.HasSuffix(key, "_ref") || strings.HasSuffix(key, "_refs") {
		return true
	}
	switch key {
	case "acceptance_checks", "agent_native_cli_checks", "baseline_commands", "commands", "evidence_required", "excluded_paths", "final_validation_commands", "forbidden_path_patterns", "forbidden_paths", "input_artifacts", "lifecycle_evidence_required", "red_first_commands", "scope_exclusions", "source", "source_ref", "source_refs", "validation_commands", "worker_evidence_required":
		return true
	default:
		return false
	}
}

func structuredPlanPathKey(key string) bool {
	switch key {
	case "affected_paths", "allowed_paths", "architecture_target_paths", "changed_paths", "file_paths", "implementation_paths", "modified_paths", "path", "paths", "planned_paths", "target_paths", "touched_paths":
		return true
	default:
		return false
	}
}

func structuredPlanProseKey(key string) bool {
	switch key {
	case "change_summary", "changes", "description", "implementation_plan", "minimum_fix", "objective", "plan", "summary":
		return true
	default:
		return false
	}
}

func quotedFieldLooksLikePathWithSpaces(parts []planField) bool {
	if len(parts) < 2 {
		return false
	}
	first := parts[0].text
	return strings.Contains(first, "/") && !knownCommandToken(first) && !strings.HasPrefix(first, "./") && !strings.HasPrefix(first, "scripts/")
}

func quotedFieldLooksLikeCommandSpan(parts []planField) bool {
	if len(parts) < 2 {
		return false
	}
	first := parts[0].text
	if knownCommandToken(first) || strings.HasPrefix(first, "./") || strings.HasPrefix(first, "scripts/") {
		return true
	}
	for _, part := range parts[1:] {
		if strings.HasPrefix(part.text, "-") || strings.Contains(part.text, "/") {
			return true
		}
	}
	return false
}

func commandContextToken(value string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(value), "-:"))
}

func commandLeadInToken(value string) bool {
	switch value {
	case "run", "execute", "executed", "invoke", "invoked", "call", "called", "test", "validate":
		return true
	default:
		return false
	}
}

func unquotedHelperExecutable(value string) bool {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), "./")
	return strings.HasPrefix(trimmed, "scripts/")
}

func knownCommandToken(value string) bool {
	switch value {
	case "bash", "go", "make", "node", "npm", "pnpm", "python", "python3", "relia", "sh", "yarn", "zsh":
		return true
	default:
		return false
	}
}

func interpreterCommandToken(value string) bool {
	switch value {
	case "bash", "node", "python", "python3", "sh", "zsh":
		return true
	default:
		return false
	}
}

func commandScriptOperand(part string) bool {
	trimmed := strings.TrimPrefix(part, "./")
	return strings.HasPrefix(trimmed, "scripts/")
}

func planQuoteAt(raw string, index int) (byte, bool) {
	switch raw[index] {
	case '"', '`':
		return raw[index], true
	case '\'':
		if index > 0 && isASCIIAlpha(rune(raw[index-1])) {
			return 0, false
		}
		return raw[index], true
	default:
		return 0, false
	}
}

func closingPlanQuoteIndex(raw string, start int, quote byte) int {
	for index := start; index < len(raw); index++ {
		if raw[index] != quote {
			continue
		}
		if quote == '\'' && index+1 < len(raw) && isASCIIAlpha(rune(raw[index+1])) {
			continue
		}
		return index
	}
	return -1
}

func recordPlanPath(touched map[string]bool, field string) {
	field = strings.TrimSpace(field)
	field = strings.TrimRight(field, "!?")
	if !strings.HasSuffix(field, "...") {
		field = strings.TrimRight(field, ".")
	}
	field = strings.TrimRight(field, ":")
	if field == "" || strings.Contains(field, "://") || githubShorthandRef(field) {
		return
	}
	if index := strings.Index(field, "#"); index >= 0 {
		field = field[:index]
	}
	field = planPathToken(field)
	if field == "" {
		return
	}
	for _, part := range strings.Split(field, ",") {
		recordPlanPathValue(touched, part)
	}
}

func recordPlanPathValue(touched map[string]bool, field string) {
	field = stripLineSuffix(strings.TrimSpace(field))
	if !planPathCandidate(field) {
		return
	}
	if cleanPath, ok := normalizedDiffPath(field, false); ok {
		touched[cleanPath] = true
	}
}

func planPathToken(field string) string {
	if envAssignmentToken(field) {
		return ""
	}
	if !strings.HasPrefix(field, "-") {
		return field
	}
	_, value, found := strings.Cut(field, "=")
	if !found {
		return ""
	}
	return value
}

func envAssignmentToken(field string) bool {
	key, value, found := strings.Cut(field, "=")
	if !found || key == "" || value == "" || strings.HasPrefix(key, "-") {
		return false
	}
	for index, char := range key {
		switch {
		case char == '_', char >= 'A' && char <= 'Z', char >= 'a' && char <= 'z':
		case index > 0 && char >= '0' && char <= '9':
		default:
			return false
		}
	}
	return true
}

func stripLineSuffix(value string) string {
	for {
		index := strings.LastIndex(value, ":")
		if index < 0 || !asciiDigitsOnly(value[index+1:]) {
			return value
		}
		value = value[:index]
	}
}

func githubShorthandRef(value string) bool {
	index := strings.LastIndex(value, "#")
	if index < 0 || index == len(value)-1 || !asciiDigitsOnly(value[index+1:]) {
		return false
	}
	parts := strings.Split(value[:index], "/")
	return len(parts) == 2 && githubShorthandPart(parts[0]) && githubShorthandPart(parts[1])
}

func githubShorthandPart(value string) bool {
	if value == "" || strings.Contains(value, ".") {
		return false
	}
	for _, char := range value {
		if isASCIIAlpha(char) || char >= '0' && char <= '9' || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func asciiDigitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
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
	if abbreviationLikeToken(field) || bareDomainLikeToken(field) {
		return false
	}
	extension := filepath.Ext(field)
	return extension != "" && hasASCIIAlpha(extension)
}

func slashPathCandidate(value string) bool {
	if remotePathLikeToken(value) || slashProseToken(value) || slashNumericToken(value) || slashTaskIDChainToken(value) {
		return false
	}
	parts := strings.Split(value, "/")
	return len(parts) >= 2 && parts[0] != ""
}

func slashNumericToken(value string) bool {
	parts := strings.Split(value, "/")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if !asciiDigitsOnly(part) {
			return false
		}
	}
	return true
}

func slashTaskIDChainToken(value string) bool {
	parts := strings.Split(value, "/")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if !taskIDLikeSegment(part) {
			return false
		}
	}
	return true
}

func taskIDLikeSegment(value string) bool {
	if value == "" {
		return false
	}
	hasUpper := false
	hasDigit := false
	for _, char := range value {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= '0' && char <= '9':
			hasDigit = true
		case char == '-' || char == '_' || char == '.':
		default:
			return false
		}
	}
	return hasUpper && hasDigit
}

func bareDomainLikeToken(value string) bool {
	if strings.ContainsAny(value, "/:") {
		return false
	}
	trimmed := strings.TrimSuffix(strings.ToLower(value), ".")
	base := strings.TrimSuffix(trimmed, filepath.Ext(trimmed))
	if base == "" || strings.ContainsAny(base, " _") {
		return false
	}
	switch filepath.Ext(trimmed) {
	case ".ai", ".app", ".biz", ".co", ".com", ".dev", ".io", ".net", ".org":
		return true
	default:
		return false
	}
}

func slashProseToken(value string) bool {
	switch strings.ToLower(value) {
	case "and/or", "ci/cd", "read/write", "write/read", "input/output", "output/input", "inputs/outputs", "outputs/inputs", "on/off", "yes/no", "before/after", "after/before", "client/server", "server/client", "frontend/backend", "backend/frontend", "producer/consumer", "consumer/producer", "request/response", "response/request", "n/a":
		return true
	default:
		return false
	}
}

func remotePathLikeToken(value string) bool {
	first, _, found := strings.Cut(value, "/")
	return found && !strings.HasPrefix(first, ".") && strings.Contains(first, ".")
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
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
}
