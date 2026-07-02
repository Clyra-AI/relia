package main

import (
	"os"
	"time"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	reviewdoc "github.com/Clyra-AI/relia/internal/review"
)

func reviewResult(args []string, start time.Time) CommandResult {
	options, parseErr := reviewdoc.ParseArgs(args)
	if parseErr != nil {
		return errorResult("review", "review", usageError(parseErr.Message), start)
	}
	wd, err := os.Getwd()
	if err != nil {
		return errorResult("review", "review", internalError("could not inspect working directory", err), start)
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return errorResult("review", "review", configError("could not locate repository root from current directory"), start)
	}
	warnings, commandErr := validateReliaConfig(root)
	if commandErr != nil {
		return errorResult("review", "review", commandErr, start)
	}
	updateOptions := reviewUpdateOptions()
	rulePath, commandErr := reviewdoc.FindRulePath(root, "memory/rules", options.Rule, updateOptions)
	if commandErr != nil {
		return errorResult("review", "review", commandErr, start)
	}
	status, commandErr := reviewdoc.UpdateRuleReview(root, rulePath, options, updateOptions)
	if commandErr != nil {
		return errorResult("review", "review", commandErr, start)
	}
	rel := displayPath(root, rulePath)
	result := passResult("review", "review", "updated memory rule review label", start, map[string]any{
		"rule":         options.Rule,
		"rule_path":    rel,
		"action":       options.Action,
		"review_label": options.Label,
		"status":       status,
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs, "schemas/memory-rule.schema.json", rel)
	result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "memory_rule", Path: rel})
	return result
}
