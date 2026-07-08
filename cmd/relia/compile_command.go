package main

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	memorydoc "github.com/Clyra-AI/relia/internal/memory"
)

const compiledBlockPath = "memory/compiled/agents-block.md"

func compileResult(args []string, start time.Time) CommandResult {
	if len(args) > 0 {
		return errorResult("compile", "compile", usageError("compile does not accept positional arguments yet"), start)
	}
	wd, err := os.Getwd()
	if err != nil {
		return errorResult("compile", "compile", internalError("could not inspect working directory", err), start)
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return errorResult("compile", "compile", configError("could not locate repository root from current directory"), start)
	}
	warnings, commandErr := validateReliaConfig(root)
	if commandErr != nil {
		return errorResult("compile", "compile", commandErr, start)
	}
	document, commandErr := readReliaConfig(root)
	if commandErr != nil {
		return errorResult("compile", "compile", commandErr, start)
	}
	settings, configErr := configdoc.CompileSettingsFromConfig(document)
	if commandErr := commandErrorFromConfig(configErr); commandErr != nil {
		return errorResult("compile", "compile", commandErr, start)
	}
	rules, commandErr := memorydoc.LoadRuleSummaries(root, memoryValidationOptions())
	if commandErr != nil {
		return errorResult("compile", "compile", commandErr, start)
	}
	selectedRules := memorydoc.SelectCompiledRules(rules, settings.MaxRules)
	block := memorydoc.RenderManagedBlock(selectedRules, memorydoc.CompileOptions{
		SchemaVersion: commandSchemaVersion,
		ReliaVersion:  reliaVersion,
		MaxRules:      settings.MaxRules,
	})
	prepared, commandErr := prepareCompiledWrites(root, settings.Targets, block)
	if commandErr != nil {
		return errorResult("compile", "compile", commandErr, start)
	}
	for _, write := range prepared {
		if !write.Changed {
			continue
		}
		if commandErr := writeAtomicRepoFile(write.Path, []byte(write.Content), write.Label); commandErr != nil {
			return errorResult("compile", "compile", commandErr, start)
		}
	}
	contexts := make([]memorydoc.CompiledContext, 0, len(settings.Targets))
	for _, target := range settings.Targets {
		contexts = append(contexts, memorydoc.CompiledContextForTarget(target, selectedRules, memorydoc.CompileOptions{
			SchemaVersion: commandSchemaVersion,
			ReliaVersion:  reliaVersion,
			MaxRules:      settings.MaxRules,
		}))
	}
	changedTargets := 0
	targetResults := make([]map[string]any, 0, len(prepared))
	for _, write := range prepared {
		if write.Changed {
			changedTargets++
		}
		targetResults = append(targetResults, map[string]any{
			"path":    write.Rel,
			"kind":    write.Kind,
			"changed": write.Changed,
		})
	}
	statusCounts := memorydoc.StatusCounts(rules)
	result := passResult("compile", "compile", "compiled Relia managed context blocks", start, map[string]any{
		"compiled_block_path":       compiledBlockPath,
		"targets":                   settings.Targets,
		"target_results":            targetResults,
		"changed_targets":           changedTargets,
		"active_rule_count":         statusCounts["active"],
		"emitted_rule_count":        len(selectedRules),
		"max_rules":                 settings.MaxRules,
		"managed_begin_marker":      memorydoc.ManagedBeginMarker,
		"managed_end_marker":        memorydoc.ManagedEndMarker,
		"compiled_contexts":         contexts,
		"agent_access_boundary":     memorydoc.CompiledAgentAccessBoundary(),
		"hosted_service_required":   false,
		"live_network_required":     false,
		"non_mcp_agent_access_path": "AGENTS.md and CLAUDE.md managed block",
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs, "schemas/compiled-context.schema.json", "schemas/memory-rule.schema.json", compiledBlockPath)
	result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "compiled_context_block", Path: compiledBlockPath})
	for _, target := range settings.Targets {
		result.EvidenceRefs = append(result.EvidenceRefs, target)
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "managed_agent_context", Path: target})
	}
	for _, rule := range selectedRules {
		result.EvidenceRefs = append(result.EvidenceRefs, rule.Path)
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "memory_rule", Path: rule.Path})
	}
	return result
}

type preparedCompileWrite struct {
	Rel     string
	Path    string
	Kind    string
	Label   string
	Content string
	Changed bool
}

func prepareCompiledWrites(root string, targets []string, block string) ([]preparedCompileWrite, *CommandError) {
	prepared := []preparedCompileWrite{}
	blockContent := block + "\n"
	blockPath := filepath.Join(root, filepath.FromSlash(compiledBlockPath))
	blockChanged, commandErr := repoFileWouldChange(blockPath, blockContent, "compiled context block")
	if commandErr != nil {
		return nil, commandErr
	}
	prepared = append(prepared, preparedCompileWrite{
		Rel:     compiledBlockPath,
		Path:    blockPath,
		Kind:    "compiled_context_block",
		Label:   "compiled context block",
		Content: blockContent,
		Changed: blockChanged,
	})
	for _, target := range targets {
		content, commandErr := readOptionalRepoFile(root, target, "managed agent context")
		if commandErr != nil {
			return nil, commandErr
		}
		next, changed, err := memorydoc.UpsertManagedBlock(content, block)
		if err != nil {
			return nil, artifactContractError("managed marker discipline failed for "+target+": "+err.Error(), target)
		}
		prepared = append(prepared, preparedCompileWrite{
			Rel:     target,
			Path:    filepath.Join(root, filepath.FromSlash(target)),
			Kind:    "managed_agent_context",
			Label:   "managed agent context",
			Content: next,
			Changed: changed,
		})
	}
	return prepared, nil
}

func repoFileWouldChange(path string, content string, label string) (bool, *CommandError) {
	current, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, internalError("could not read existing "+label, err)
	}
	return string(current) != content, nil
}

func readOptionalRepoFile(root string, rel string, label string) (string, *CommandError) {
	path := filepath.Join(root, filepath.FromSlash(rel))
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", internalError("could not read existing "+label, err)
	}
	return string(content), nil
}
