package main

import (
	"os"
	"strings"
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
	mergedIntoPath := ""
	if options.Action == "merge" {
		mergedIntoPath, commandErr = reviewdoc.FindRulePath(root, "memory/rules", options.MergeInto, updateOptions)
		if commandErr != nil {
			return errorResult("review", "review", commandErr, start)
		}
		if mergedIntoPath == rulePath {
			return errorResult("review", "review", usageError("review merge --into must reference a different rule"), start)
		}
		if commandErr := validateMemoryRuleArtifact(root, mergedIntoPath); commandErr != nil {
			return errorResult("review", "review", commandErr, start)
		}
		if !isMemoryRulesPath(root, mergedIntoPath) {
			return errorResult("review", "review", usageError("review merge --into must reference a rule under memory/rules"), start)
		}
	}
	status, commandErr := reviewdoc.UpdateRuleReview(root, rulePath, options, updateOptions)
	if commandErr != nil {
		return errorResult("review", "review", commandErr, start)
	}
	rel := displayPath(root, rulePath)
	data := map[string]any{
		"rule":         options.Rule,
		"rule_path":    rel,
		"action":       options.Action,
		"review_label": options.Label,
		"review_gate":  "human_review",
		"decision":     reviewDecisionOutput(options),
		"status":       status,
	}
	if mergedIntoPath != "" {
		data["merged_into"] = options.MergeInto
		data["merged_into_path"] = displayPath(root, mergedIntoPath)
	}
	result := passResult("review", "review", "updated memory rule review label", start, data)
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs, "schemas/memory-rule.schema.json", rel)
	result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "memory_rule", Path: rel})
	return result
}

func reviewDecisionOutput(options reviewdoc.Options) string {
	switch options.Action {
	case "approve":
		return "approved"
	case "reject":
		return "rejected"
	case "merge":
		return "merged"
	case "edit":
		return "pending"
	default:
		switch options.Label {
		case "accepted":
			return "approved"
		case "needs_user_input":
			return "needs_user_input"
		default:
			return "pending"
		}
	}
}

func isMemoryRulesPath(root string, path string) bool {
	return strings.HasPrefix(displayPath(root, path), "memory/rules/")
}
