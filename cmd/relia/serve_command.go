package main

import (
	"os"
	"time"

	assessdoc "github.com/Clyra-AI/relia/internal/assess"
	configdoc "github.com/Clyra-AI/relia/internal/config"
	servedoc "github.com/Clyra-AI/relia/internal/serve"
)

func serveResult(args []string, start time.Time) CommandResult {
	options, parseErr := servedoc.ParseArgs(args)
	withFormat := func(result CommandResult) CommandResult {
		if options.Format == "json" {
			result.MachineReadable = true
		}
		return result
	}
	if parseErr != nil {
		commandErr := usageError(parseErr.Message)
		if parseErr.Kind == servedoc.ErrorKindDependency {
			commandErr = dependencyError(parseErr.Message, parseErr.Reference)
		}
		return withFormat(errorResult("serve", "serve", commandErr, start))
	}
	wd, err := os.Getwd()
	if err != nil {
		return withFormat(errorResult("serve", "serve", internalError("could not inspect working directory", err), start))
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return withFormat(errorResult("serve", "serve", configError("could not locate repository root from current directory"), start))
	}
	warnings, commandErr := validateReliaConfig(root)
	if commandErr != nil {
		return withFormat(errorResult("serve", "serve", commandErr, start))
	}
	rules, commandErr := assessdoc.LoadRules(root, assessmentBuildOptions())
	if commandErr != nil {
		return withFormat(errorResult("serve", "serve", commandErr, start))
	}
	servedRules, commandErr := servedRuleData(rules)
	if commandErr != nil {
		return withFormat(errorResult("serve", "serve", commandErr, start))
	}
	result := passResult("serve", "serve", "exposed local MCP capability manifest for active memory rules", start, map[string]any{
		"format":                  options.Format,
		"mcp":                     map[string]any{"transport": "stdio", "tools": []string{"recall", "assess", "coverage"}},
		"active_rule_count":       len(rules),
		"served_rules":            servedRules,
		"hosted_service_required": false,
		"live_network_required":   false,
		"advisory_only":           true,
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs, "schemas/risk-assessment.schema.json", "schemas/memory-rule.schema.json")
	for _, rule := range rules {
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "memory_rule", Path: rule.Path})
		result.EvidenceRefs = append(result.EvidenceRefs, rule.Path)
	}
	return withFormat(result)
}

func servedRuleData(rules []assessdoc.Rule) ([]map[string]any, *CommandError) {
	return assessdoc.ServedRuleData(rules, assessmentBuildOptions())
}
