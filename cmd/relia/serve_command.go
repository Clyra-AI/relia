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
	if options.Tool != "" {
		return withFormat(serveToolResult(root, options, rules, start, warnings))
	}
	servedRules, commandErr := servedRuleData(rules)
	if commandErr != nil {
		return withFormat(errorResult("serve", "serve", commandErr, start))
	}
	result := passResult("serve", "serve", "exposed local MCP capability manifest for active memory rules", start, map[string]any{
		"format":                  options.Format,
		"mcp":                     servedoc.Manifest(len(rules), servedRules),
		"active_rule_count":       len(rules),
		"served_rules":            servedRules,
		"agent_access_boundary":   servedoc.AgentAccessBoundary(),
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

func serveToolResult(root string, options servedoc.Options, rules []assessdoc.Rule, start time.Time, warnings []Finding) CommandResult {
	resultData := map[string]any{
		"format":                  options.Format,
		"tool_name":               options.Tool,
		"mcp":                     servedoc.Manifest(len(rules), nil),
		"active_rule_count":       len(rules),
		"agent_access_boundary":   servedoc.AgentAccessBoundary(),
		"hosted_service_required": false,
		"live_network_required":   false,
		"advisory_only":           true,
	}
	var toolResult map[string]any
	var commandErr *CommandError
	switch options.Tool {
	case "recall":
		toolResult, commandErr = servedoc.BuildRecallResult(root, options, rules, assessmentBuildOptions())
	case "coverage":
		toolResult, commandErr = servedoc.BuildCoverageResult(root, options.Paths, rules, assessmentBuildOptions())
	case "assess":
		toolResult, commandErr = serveAssessToolResult(root, options, rules)
	default:
		commandErr = usageError("serve --tool must be one of recall, assess, or coverage")
	}
	if commandErr != nil {
		return errorResultWithData("serve", "serve", commandErr, start, resultData)
	}
	resultData["tool_result"] = toolResult
	result := passResult("serve", "serve", "served local MCP "+options.Tool+" tool response", start, resultData)
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs,
		"schemas/risk-assessment.schema.json",
		"schemas/coverage-map.schema.json",
		"schemas/memory-rule.schema.json",
	)
	for _, rule := range rules {
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "memory_rule", Path: rule.Path})
		result.EvidenceRefs = append(result.EvidenceRefs, rule.Path)
	}
	if options.Tool == "assess" && options.InputPath != "" {
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "input_diff", Path: options.InputPath})
		result.EvidenceRefs = append(result.EvidenceRefs, options.InputPath)
	}
	return result
}

func serveAssessToolResult(root string, options servedoc.Options, rules []assessdoc.Rule) (map[string]any, *CommandError) {
	inputPath := resolveInputPath(root, options.InputPath)
	inputContent, err := os.ReadFile(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, artifactContractError("serve assess input diff is missing", displayPath(root, inputPath))
		}
		return nil, internalError("could not read serve assess input", err)
	}
	touchedPaths, commandErr := parseUnifiedDiffTouchedPaths(inputContent, displayPath(root, inputPath))
	if commandErr != nil {
		return nil, commandErr
	}
	assessment, commandErr := assessdoc.BuildRiskAssessment(root, displayPath(root, inputPath), inputContent, touchedPaths, rules, assessmentBuildOptions())
	if commandErr != nil {
		return nil, commandErr
	}
	return map[string]any{
		"object_type":           "relia.mcp_assess_response",
		"schema_version":        commandSchemaVersion,
		"input_path":            displayPath(root, inputPath),
		"touched_paths":         touchedPaths,
		"assessment":            assessment,
		"matched_rule_count":    len(assessment.Matches),
		"agent_access_boundary": servedoc.AgentAccessBoundary(),
		"metadata": map[string]any{
			"engine":                   "relia.assess",
			"repo_relative_paths_only": true,
			"active_memory_only":       true,
		},
	}, nil
}
