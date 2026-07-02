package main

import (
	"errors"
	"os"
	"time"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

func ingestResult(args []string, start time.Time) CommandResult {
	options, parseErr := ingestdoc.ParseArgs(args)
	if parseErr != nil {
		return errorResult("ingest", "ingest", usageError(parseErr.Message), start)
	}
	wd, err := os.Getwd()
	if err != nil {
		return errorResult("ingest", "ingest", internalError("could not inspect working directory", err), start)
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return errorResult("ingest", "ingest", configError("could not locate repository root from current directory"), start)
	}
	warnings, commandErr := validateReliaConfig(root)
	if commandErr != nil {
		return errorResult("ingest", "ingest", commandErr, start)
	}
	config, commandErr := readReliaConfig(root)
	if commandErr != nil {
		return errorResult("ingest", "ingest", commandErr, start)
	}
	inputPath := resolveInputPath(root, options.InputPath)
	inputContent, err := os.ReadFile(inputPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errorResult("ingest", "ingest", artifactContractError("ingest input is missing", displayPath(root, inputPath)), start)
		}
		return errorResult("ingest", "ingest", internalError("could not read ingest input", err), start)
	}
	events, commandErr := parseIngestEvents(inputContent, displayPath(root, inputPath))
	if commandErr != nil {
		return errorResult("ingest", "ingest", commandErr, start)
	}

	records := make([]experienceRecord, 0, len(events))
	skippedUncertain := 0
	agentAttributed := 0
	humanAttributed := 0
	ingestedAt := start.UTC().Format(time.RFC3339)
	for index, event := range events {
		redacted, commandErr := redactForPersistence(event, displayPath(root, inputPath))
		if commandErr != nil {
			return errorResult("ingest", "ingest", commandErr, start)
		}
		redactedEvent, ok := redacted.(map[string]any)
		if !ok {
			return errorResult("ingest", "ingest", artifactContractError("ingest event must be a JSON object", displayPath(root, inputPath)), start)
		}
		record, skipped, commandErr := normalizeExperienceRecord(config, redactedEvent, index, displayPath(root, inputPath))
		if commandErr != nil {
			return errorResult("ingest", "ingest", commandErr, start)
		}
		if skipped {
			skippedUncertain++
			continue
		}
		if record.Metadata == nil {
			record.Metadata = map[string]any{}
		}
		record.Metadata[badgeMetadataLastIngest] = ingestedAt
		record.Metadata[badgeMetadataMergedPRs] = 0
		switch record.Attribution.ActorKind {
		case "agent":
			agentAttributed++
		case "human":
			humanAttributed++
		}
		records = append(records, record)
	}

	shards, commandErr := persistExperienceRecords(root, records)
	if commandErr != nil {
		return errorResult("ingest", "ingest", commandErr, start)
	}
	result := passResult("ingest", "ingest", "ingested canonical experience records", start, map[string]any{
		"input_path":                    displayPath(root, inputPath),
		"experiences_total":             len(events),
		"experiences_persisted":         len(records),
		"experiences_agent_attributed":  agentAttributed,
		"experiences_human_attributed":  humanAttributed,
		"experiences_skipped_uncertain": skippedUncertain,
		"artifact_root":                 ".relia",
		"commit_experiences":            false,
		"experience_shards":             shards,
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.RedactionStatus = "applied"
	result.EvidenceRefs = append(result.EvidenceRefs,
		"docs/product/prd.md#2-ingest",
		"schemas/experience-record.schema.json",
	)
	result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "input", Path: displayPath(root, inputPath)})
	for _, shard := range shards {
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "experience_shard", Path: shard})
	}
	return result
}
