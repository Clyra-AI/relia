package review

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	memorydoc "github.com/Clyra-AI/relia/internal/memory"
	resultdoc "github.com/Clyra-AI/relia/internal/result"
	"github.com/Clyra-AI/relia/internal/yamlmini"
)

type UpdateOptions struct {
	SchemaVersion         string
	UsageError            func(string) *resultdoc.CommandError
	ArtifactContractError func(string, string) *resultdoc.CommandError
	InternalError         func(string, error) *resultdoc.CommandError
	RepoPathExists        func(string, string) bool
	YAMLFloat             func(float64) string
}

func FindRulePath(root string, ruleDir string, rule string, options UpdateOptions) (string, *resultdoc.CommandError) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "", usageError(options, "memory rule id or path must be non-empty")
	}
	if clean, ok := cleanRepoPath(rule); ok && (strings.HasSuffix(clean, ".yaml") || strings.HasSuffix(clean, ".yml")) {
		path := filepath.Join(root, filepath.FromSlash(filepath.ToSlash(clean)))
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	patterns := []string{
		filepath.Join(root, filepath.FromSlash(ruleDir), "*.yaml"),
		filepath.Join(root, filepath.FromSlash(ruleDir), "*.yml"),
	}
	for _, pattern := range patterns {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			return "", internalError(options, "could not inspect memory rules", err)
		}
		sort.Strings(paths)
		for _, path := range paths {
			content, err := os.ReadFile(path)
			if err != nil {
				return "", internalError(options, "could not read memory rule artifact", err)
			}
			document, parseErr := yamlmini.ParseDocument(string(content))
			if parseErr != nil {
				return "", artifactContractError(options, parseErr.Error(), displayPath(root, path))
			}
			if document.Scalars["id"].Value == rule {
				return path, nil
			}
		}
	}
	return "", artifactContractError(options, "memory rule was not found", rule)
}

func UpdateRuleReview(root string, rulePath string, reviewOptions Options, updateOptions UpdateOptions) (string, *resultdoc.CommandError) {
	if commandErr := memorydoc.ValidateRuleArtifact(root, rulePath, memoryValidationOptions(updateOptions)); commandErr != nil {
		return "", commandErr
	}
	content, err := os.ReadFile(rulePath)
	if err != nil {
		return "", internalError(updateOptions, "could not read memory rule artifact", err)
	}
	rel := displayPath(root, rulePath)
	document, parseErr := yamlmini.ParseDocument(string(content))
	if parseErr != nil {
		return "", artifactContractError(updateOptions, parseErr.Error(), rel)
	}
	status := document.Scalars["status"].Value
	label := reviewOptions.Label
	next := string(content)
	switch reviewOptions.Action {
	case "approve":
		switch status {
		case "stale", "contradicted", "retired":
			return "", artifactContractError(updateOptions, "cannot mark "+status+" memory rule accepted without fresh distill evidence", rel)
		}
		status = "active"
		label = "accepted"
		next = applyReviewDecisionYAML(next, reviewOptions, "approved", "")
		next = replaceOrAddNestedYAMLScalar(next, "metadata", "lifecycle_reason", "approved by human review")
	case "reject":
		status = "retired"
		label = "needs_user_input"
		next = applyReviewDecisionYAML(next, reviewOptions, "rejected", "")
		next = replaceOrAddNestedYAMLScalar(next, "metadata", "lifecycle_reason", "rejected by human review: "+strings.TrimSpace(reviewOptions.Reason))
	case "edit":
		switch status {
		case "stale", "contradicted", "retired":
			return "", artifactContractError(updateOptions, "cannot edit "+status+" memory rule without fresh distill evidence", rel)
		}
		status = "candidate"
		if strings.TrimSpace(reviewOptions.Statement) != "" {
			next = replaceTopLevelYAMLScalar(next, "statement", strings.TrimSpace(reviewOptions.Statement))
			next = replaceNestedYAMLScalar(next, "review", "statement_origin", "human_authored")
		}
		if len(reviewOptions.ScopePaths) > 0 {
			scopePaths := normalizedRepoPaths(reviewOptions.ScopePaths)
			for _, scopePath := range scopePaths {
				if !repoPathExists(updateOptions, root, scopePath) {
					return "", artifactContractError(updateOptions, "memory rule scope path does not exist in the repo", rel)
				}
			}
			next = replaceNestedYAMLStringList(next, "scope", "paths", scopePaths)
		}
		next = applyReviewDecisionYAML(next, reviewOptions, "pending", "")
		next = replaceOrAddNestedYAMLScalar(next, "metadata", "lifecycle_reason", "edited by human review; pending approval")
	case "merge":
		status = "retired"
		label = "needs_user_input"
		next = applyReviewDecisionYAML(next, reviewOptions, "merged", strings.TrimSpace(reviewOptions.MergeInto))
		next = replaceOrAddNestedYAMLScalar(next, "metadata", "lifecycle_reason", "merged by human review into "+strings.TrimSpace(reviewOptions.MergeInto)+": "+strings.TrimSpace(reviewOptions.Reason))
	case "label":
		if label == "accepted" {
			switch status {
			case "stale", "contradicted", "retired":
				return "", artifactContractError(updateOptions, "cannot mark "+status+" memory rule accepted without fresh distill evidence", rel)
			}
			status = "active"
			next = applyReviewDecisionYAML(next, reviewOptions, "approved", "")
			next = replaceOrAddNestedYAMLScalar(next, "metadata", "lifecycle_reason", "approved by human review")
		} else if status == "active" {
			status = "candidate"
			next = applyReviewDecisionYAML(next, reviewOptions, reviewDecisionForLabel(label), "")
			next = replaceOrAddNestedYAMLScalar(next, "metadata", "lifecycle_reason", "returned to candidate review")
		} else {
			next = applyReviewDecisionYAML(next, reviewOptions, reviewDecisionForLabel(label), "")
		}
	default:
		return "", usageError(updateOptions, "review action must be approve, edit, merge, reject, or omitted for --label")
	}
	next = replaceTopLevelYAMLScalar(next, "status", status)
	next = replaceNestedYAMLScalar(next, "review", "label", label)
	if next == string(content) {
		return "", artifactContractError(updateOptions, "memory rule review fields were not updated", rel)
	}
	if commandErr := writeAtomicFile(rulePath, []byte(next), "memory rule", updateOptions); commandErr != nil {
		return "", commandErr
	}
	if commandErr := memorydoc.ValidateRuleArtifact(root, rulePath, memoryValidationOptions(updateOptions)); commandErr != nil {
		return "", commandErr
	}
	return status, nil
}

func applyReviewDecisionYAML(content string, reviewOptions Options, decision string, mergedInto string) string {
	next := replaceOrAddNestedYAMLScalar(content, "review", "gate", "human_review")
	next = replaceOrAddNestedYAMLScalar(next, "review", "decision", decision)
	reviewer := strings.TrimSpace(reviewOptions.ReviewedBy)
	if reviewer == "" {
		reviewer = "maintainer"
	}
	next = replaceOrAddNestedYAMLScalar(next, "review", "reviewed_by", reviewer)
	next = replaceOrAddNestedYAMLScalar(next, "review", "decision_ref", reviewDecisionRef(reviewOptions, decision))
	if strings.TrimSpace(mergedInto) != "" {
		next = replaceOrAddNestedYAMLScalar(next, "review", "merged_into", strings.TrimSpace(mergedInto))
	} else {
		next = removeNestedYAMLScalar(next, "review", "merged_into")
	}
	return next
}

func reviewDecisionForLabel(label string) string {
	switch label {
	case "accepted":
		return "approved"
	case "needs_user_input":
		return "needs_user_input"
	default:
		return "pending"
	}
}

func reviewDecisionRef(reviewOptions Options, decision string) string {
	action := reviewOptions.Action
	if action == "label" {
		action = "--label " + reviewOptions.Label
	} else if action == "merge" {
		action = "merge --into " + strings.TrimSpace(reviewOptions.MergeInto)
	}
	if action == "" {
		action = decision
	}
	return "relia review " + action + " --rule " + strings.TrimSpace(reviewOptions.Rule)
}

func replaceTopLevelYAMLScalar(content string, key string, value string) string {
	lines := strings.Split(content, "\n")
	prefix := key + ":"
	for index, line := range lines {
		if yamlmini.LeadingSpaces(line) == 0 && strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[index] = key + ": " + yamlScalarForWrite(value)
			break
		}
	}
	return strings.Join(lines, "\n")
}

func replaceNestedYAMLScalar(content string, parent string, key string, value string) string {
	lines := strings.Split(content, "\n")
	inParent := false
	parentPrefix := parent + ":"
	keyPrefix := key + ":"
	for index, line := range lines {
		indent := yamlmini.LeadingSpaces(line)
		trimmed := strings.TrimSpace(line)
		if indent == 0 {
			inParent = strings.HasPrefix(trimmed, parentPrefix)
			continue
		}
		if inParent && indent == 2 && strings.HasPrefix(trimmed, keyPrefix) {
			lines[index] = "  " + key + ": " + yamlScalarForWrite(value)
			break
		}
	}
	return strings.Join(lines, "\n")
}

func removeNestedYAMLScalar(content string, parent string, key string) string {
	lines := strings.Split(content, "\n")
	parentPrefix := parent + ":"
	keyPrefix := key + ":"
	inParent := false
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		indent := yamlmini.LeadingSpaces(line)
		trimmed := strings.TrimSpace(line)
		if indent == 0 {
			inParent = strings.HasPrefix(trimmed, parentPrefix)
		}
		if inParent && indent == 2 && strings.HasPrefix(trimmed, keyPrefix) {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func replaceOrAddNestedYAMLScalar(content string, parent string, key string, value string) string {
	lines := strings.Split(content, "\n")
	parentPrefix := parent + ":"
	keyPrefix := key + ":"
	parentIndex := -1
	insertIndex := len(lines)
	inParent := false
	for index, line := range lines {
		indent := yamlmini.LeadingSpaces(line)
		trimmed := strings.TrimSpace(line)
		if indent == 0 {
			if inParent {
				insertIndex = index
				break
			}
			inParent = strings.HasPrefix(trimmed, parentPrefix)
			if inParent {
				parentIndex = index
				insertIndex = index + 1
			}
			continue
		}
		if inParent {
			insertIndex = index + 1
			if indent == 2 && strings.HasPrefix(trimmed, keyPrefix) {
				lines[index] = "  " + key + ": " + yamlScalarForWrite(value)
				return strings.Join(lines, "\n")
			}
		}
	}
	newLine := "  " + key + ": " + yamlScalarForWrite(value)
	if parentIndex == -1 {
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines[len(lines)-1] = parentPrefix
			lines = append(lines, newLine, "")
			return strings.Join(lines, "\n")
		}
		lines = append(lines, parentPrefix, newLine)
		return strings.Join(lines, "\n")
	}
	next := append([]string{}, lines[:insertIndex]...)
	next = append(next, newLine)
	next = append(next, lines[insertIndex:]...)
	return strings.Join(next, "\n")
}

func replaceNestedYAMLStringList(content string, parent string, key string, values []string) string {
	lines := strings.Split(content, "\n")
	parentPrefix := parent + ":"
	keyPrefix := key + ":"
	inParent := false
	parentIndex := -1
	insertStart := -1
	insertEnd := -1
	for index, line := range lines {
		indent := yamlmini.LeadingSpaces(line)
		trimmed := strings.TrimSpace(line)
		if indent == 0 {
			if inParent {
				if insertStart < 0 {
					insertStart = index
					insertEnd = index
				}
				break
			}
			inParent = strings.HasPrefix(trimmed, parentPrefix)
			if inParent {
				parentIndex = index
			}
			continue
		}
		if !inParent {
			continue
		}
		if indent == 2 && strings.HasPrefix(trimmed, keyPrefix) {
			insertStart = index
			insertEnd = index + 1
			for insertEnd < len(lines) {
				nextLine := lines[insertEnd]
				nextIndent := yamlmini.LeadingSpaces(nextLine)
				nextTrimmed := strings.TrimSpace(nextLine)
				if nextTrimmed != "" && nextIndent <= 2 {
					break
				}
				insertEnd++
			}
			break
		}
	}
	replacement := []string{"  " + key + ":"}
	for _, value := range values {
		replacement = append(replacement, "    - "+yamlScalarForWrite(value))
	}
	if insertStart == -1 {
		if parentIndex == -1 {
			return content
		}
		insertStart = parentIndex + 1
		insertEnd = insertStart
	}
	next := append([]string{}, lines[:insertStart]...)
	next = append(next, replacement...)
	next = append(next, lines[insertEnd:]...)
	return strings.Join(next, "\n")
}

func normalizedRepoPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, value := range paths {
		if clean, ok := cleanRepoPath(value); ok {
			clean = filepath.ToSlash(clean)
			if !seen[clean] {
				seen[clean] = true
				result = append(result, clean)
			}
		}
	}
	sort.Strings(result)
	return result
}

func writeAtomicFile(path string, content []byte, label string, options UpdateOptions) *resultdoc.CommandError {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return internalError(options, "could not create "+label+" directory", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return internalError(options, "could not create temporary "+label, err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return internalError(options, "could not write temporary "+label, err)
	}
	if err := tempFile.Close(); err != nil {
		return internalError(options, "could not close temporary "+label, err)
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return internalError(options, "could not set temporary "+label+" permissions", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return internalError(options, "could not write "+label, err)
	}
	cleanup = false
	return nil
}

func memoryValidationOptions(options UpdateOptions) memorydoc.ValidationOptions {
	return memorydoc.ValidationOptions{
		SchemaVersion:         options.SchemaVersion,
		ArtifactContractError: options.ArtifactContractError,
		InternalError:         options.InternalError,
		RepoPathExists:        options.RepoPathExists,
		YAMLFloat:             options.YAMLFloat,
	}
}

func repoPathExists(options UpdateOptions, root string, rel string) bool {
	if options.RepoPathExists != nil {
		return options.RepoPathExists(root, rel)
	}
	clean, ok := cleanRepoPath(rel)
	if !ok {
		return false
	}
	_, err := os.Stat(filepath.Join(root, clean))
	return err == nil
}

func displayPath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func yamlScalarForWrite(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, "\n\r#") ||
		strings.Contains(value, ": ") ||
		strings.HasPrefix(value, "-") ||
		strings.HasPrefix(value, "{") ||
		strings.HasPrefix(value, "[") ||
		strings.HasSuffix(value, ":") ||
		value == "true" ||
		value == "false" {
		return strconv.Quote(value)
	}
	return value
}

func usageError(options UpdateOptions, message string) *resultdoc.CommandError {
	if options.UsageError != nil {
		return options.UsageError(message)
	}
	return &resultdoc.CommandError{Type: "invalid_usage", Message: message}
}

func artifactContractError(options UpdateOptions, message string, ref string) *resultdoc.CommandError {
	if options.ArtifactContractError != nil {
		return options.ArtifactContractError(message, ref)
	}
	return &resultdoc.CommandError{Type: "artifact_contract_validation_failed", Message: message, Ref: ref}
}

func internalError(options UpdateOptions, message string, err error) *resultdoc.CommandError {
	if options.InternalError != nil {
		return options.InternalError(message, err)
	}
	if err != nil {
		message += ": " + err.Error()
	}
	return &resultdoc.CommandError{Type: "internal_error", Message: message}
}
