package main

import (
	"os"
	"time"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	memorydoc "github.com/Clyra-AI/relia/internal/memory"
)

func memoryResult(args []string, start time.Time) CommandResult {
	options, parseErr := memorydoc.ParseArgs(args)
	withFormat := func(result CommandResult) CommandResult {
		if options.Format == "json" {
			result.MachineReadable = true
		}
		return result
	}
	if parseErr != nil {
		return withFormat(errorResult("memory", "memory", usageError(parseErr.Message), start))
	}
	wd, err := os.Getwd()
	if err != nil {
		return withFormat(errorResult("memory", "memory", internalError("could not inspect working directory", err), start))
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return withFormat(errorResult("memory", "memory", configError("could not locate repository root from current directory"), start))
	}
	warnings, commandErr := validateReliaConfig(root)
	if commandErr != nil {
		return withFormat(errorResult("memory", "memory", commandErr, start))
	}
	rules, commandErr := memorydoc.LoadRuleSummaries(root, memoryValidationOptions())
	if commandErr != nil {
		return withFormat(errorResult("memory", "memory", commandErr, start))
	}
	outputPath, commandErr := writeMemoryPage(root, options.OutputPath, rules)
	if commandErr != nil {
		return withFormat(errorResult("memory", "memory", commandErr, start))
	}
	statusCounts := memorydoc.StatusCounts(rules)
	result := passResult("memory", "memory", "rendered MEMORY.md with rule receipts", start, map[string]any{
		"format":           options.Format,
		"memory_page_path": outputPath,
		"rule_count":       len(rules),
		"active_rules":     statusCounts["active"],
		"candidate_rules":  statusCounts["candidate"],
		"stale_rules":      statusCounts["stale"],
		"contradicted":     statusCounts["contradicted"],
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs, "schemas/memory-rule.schema.json", outputPath)
	result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "memory_page", Path: outputPath})
	for _, rule := range rules {
		result.EvidenceRefs = append(result.EvidenceRefs, rule.Path)
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "memory_rule", Path: rule.Path})
	}
	return withFormat(result)
}
