package main

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	advisedoc "github.com/Clyra-AI/relia/internal/advise"
	assessdoc "github.com/Clyra-AI/relia/internal/assess"
	configdoc "github.com/Clyra-AI/relia/internal/config"
)

func adviseResult(args []string, start time.Time) CommandResult {
	options, parseErr := advisedoc.ParseArgs(args)
	withFormat := func(result CommandResult) CommandResult {
		if options.Format == "json" {
			result.MachineReadable = true
		}
		return result
	}
	if parseErr != nil {
		return withFormat(errorResult("advise", "advise", usageError(parseErr.Message), start))
	}
	wd, err := os.Getwd()
	if err != nil {
		return withFormat(errorResult("advise", "advise", internalError("could not inspect working directory", err), start))
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return withFormat(errorResult("advise", "advise", configError("could not locate repository root from current directory"), start))
	}
	warnings, commandErr := validateReliaConfig(root)
	if commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	config, commandErr := readReliaConfig(root)
	if commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	settings, commandErr := adviseSettingsFromConfig(config)
	if commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	inputPath := resolveInputPath(root, options.InputPath)
	inputContent, err := os.ReadFile(inputPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return withFormat(errorResult("advise", "advise", artifactContractError("advise input diff is missing", displayPath(root, inputPath)), start))
		}
		return withFormat(errorResult("advise", "advise", internalError("could not read advise input", err), start))
	}
	touchedPaths, inputKind, commandErr := parseAssessmentInputPaths(inputContent, displayPath(root, inputPath))
	if commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	rules, commandErr := assessdoc.LoadRules(root, assessmentBuildOptions())
	if commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	assessment, commandErr := assessdoc.BuildRiskAssessment(root, displayPath(root, inputPath), inputContent, touchedPaths, rules, assessmentBuildOptions(), inputKind)
	if commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	diffFingerprint := sha256String(string(inputContent))
	previousState, stateErr := advisedoc.LoadPriorState(root, options.StatePath)
	if stateErr != nil {
		return withFormat(errorResult("advise", "advise", commandErrorFromAdviseState(stateErr), start))
	}
	shouldComment, skipReason := advisedoc.CommentDecision(settings, assessment, diffFingerprint, previousState, start)
	forwardBaseline, stateErr := advisedoc.LoadForwardBaseline(root, options.BaselinePath)
	if stateErr != nil {
		return withFormat(errorResult("advise", "advise", commandErrorFromAdviseState(stateErr), start))
	}
	forwardSignal := advisedoc.BuildForwardSignal(commandSchemaVersion, displayPath(root, inputPath), assessment, settings, diffFingerprint, forwardBaseline, shouldComment, skipReason, start)
	body := ""
	if shouldComment {
		body = advisedoc.RenderComment(assessment, touchedPaths, diffFingerprint, start, skipReason)
		if commandErr := writeRepoRelativeFile(root, options.CommentPath, []byte(body), "advisory comment"); commandErr != nil {
			return withFormat(errorResult("advise", "advise", commandErr, start))
		}
	}
	state := advisedoc.StateDocument(
		commandSchemaVersion,
		displayPath(root, inputPath),
		assessment,
		settings,
		diffFingerprint,
		previousState,
		shouldComment,
		skipReason,
		start,
		forwardSignal,
	)
	encodedState, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return withFormat(errorResult("advise", "advise", internalError("could not encode advisory state", err), start))
	}
	if commandErr := writeRepoRelativeFile(root, options.StatePath, append(encodedState, '\n'), "advisory state"); commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	result := passResult("advise", "advise", "planned advisory PR comment from local assessment", start, map[string]any{
		"input_path":         displayPath(root, inputPath),
		"input_kind":         inputKind,
		"format":             options.Format,
		"touched_paths":      touchedPaths,
		"active_rule_count":  len(rules),
		"matched_rule_count": len(assessment.Matches),
		"assessment":         assessment,
		"diff_fingerprint":   diffFingerprint,
		"should_comment":     shouldComment,
		"skip_reason":        skipReason,
		"comment_path":       options.CommentPath,
		"state_path":         options.StatePath,
		"baseline_path":      options.BaselinePath,
		"forward_signal":     forwardSignal,
		"comment_strategy":   advisedoc.CommentStrategy(settings),
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs,
		"schemas/risk-assessment.schema.json",
		displayPath(root, inputPath),
		options.StatePath,
		options.BaselinePath,
	)
	result.Artifacts = append(result.Artifacts,
		ArtifactRef{Kind: "input_diff", Path: displayPath(root, inputPath)},
		ArtifactRef{Kind: "advisory_state", Path: options.StatePath},
	)
	if shouldComment {
		result.EvidenceRefs = append(result.EvidenceRefs, options.CommentPath)
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "advisory_comment", Path: options.CommentPath})
	}
	for _, rule := range rules {
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "memory_rule", Path: rule.Path})
		result.EvidenceRefs = append(result.EvidenceRefs, rule.Path)
	}
	return withFormat(result)
}
