package main

import (
	"errors"
	"os"
	"time"

	assessdoc "github.com/Clyra-AI/relia/internal/assess"
	configdoc "github.com/Clyra-AI/relia/internal/config"
)

func assessResult(args []string, start time.Time) CommandResult {
	options, parseErr := assessdoc.ParseArgs(args)
	withFormat := func(result CommandResult) CommandResult {
		if options.Format == "json" {
			result.MachineReadable = true
		}
		return result
	}
	if parseErr != nil {
		return withFormat(errorResult("assess", "assess", usageError(parseErr.Message), start))
	}
	wd, err := os.Getwd()
	if err != nil {
		return withFormat(errorResult("assess", "assess", internalError("could not inspect working directory", err), start))
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return withFormat(errorResult("assess", "assess", configError("could not locate repository root from current directory"), start))
	}
	warnings, commandErr := validateReliaConfig(root)
	if commandErr != nil {
		return withFormat(errorResult("assess", "assess", commandErr, start))
	}

	inputPath := resolveInputPath(root, options.InputPath)
	inputContent, err := os.ReadFile(inputPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return withFormat(errorResult("assess", "assess", artifactContractError("assess input diff is missing", displayPath(root, inputPath)), start))
		}
		return withFormat(errorResult("assess", "assess", internalError("could not read assess input", err), start))
	}
	touchedPaths, commandErr := parseUnifiedDiffTouchedPaths(inputContent, displayPath(root, inputPath))
	if commandErr != nil {
		return withFormat(errorResult("assess", "assess", commandErr, start))
	}
	rules, commandErr := assessdoc.LoadRules(root, assessmentBuildOptions())
	if commandErr != nil {
		return withFormat(errorResult("assess", "assess", commandErr, start))
	}
	assessment, commandErr := assessdoc.BuildRiskAssessment(root, displayPath(root, inputPath), inputContent, touchedPaths, rules, assessmentBuildOptions())
	if commandErr != nil {
		return withFormat(errorResult("assess", "assess", commandErr, start))
	}

	result := passResult("assess", "assess", "assessed local diff against active memory rules", start, map[string]any{
		"input_path":         displayPath(root, inputPath),
		"format":             options.Format,
		"touched_paths":      touchedPaths,
		"active_rule_count":  len(rules),
		"matched_rule_count": len(assessment.Matches),
		"assessment":         assessment,
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs,
		"schemas/risk-assessment.schema.json",
		displayPath(root, inputPath),
	)
	result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "input_diff", Path: displayPath(root, inputPath)})
	for _, rule := range rules {
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "memory_rule", Path: rule.Path})
		result.EvidenceRefs = append(result.EvidenceRefs, rule.Path)
	}
	return withFormat(result)
}
