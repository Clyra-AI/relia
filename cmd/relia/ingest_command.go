package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

const (
	githubLiveOverallTimeout     = 2 * time.Minute
	githubLiveHTTPRequestTimeout = 30 * time.Second
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
	events, sourceRef, liveReceipt, commandErr := ingestEventsForOptions(root, options)
	if commandErr != nil {
		return errorResult("ingest", "ingest", commandErr, start)
	}

	records := make([]experienceRecord, 0, len(events))
	skippedUncertain := 0
	agentAttributed := 0
	humanAttributed := 0
	ingestedAt := start.UTC().Format(time.RFC3339)
	for index, event := range events {
		redacted, commandErr := redactForPersistence(event, sourceRef)
		if commandErr != nil {
			return errorResult("ingest", "ingest", commandErr, start)
		}
		redactedEvent, ok := redacted.(map[string]any)
		if !ok {
			return errorResult("ingest", "ingest", artifactContractError("ingest event must be a JSON object", sourceRef), start)
		}
		record, skipped, commandErr := normalizeExperienceRecord(config, redactedEvent, index, sourceRef)
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
	data := map[string]any{
		"source_ref":                    sourceRef,
		"source_format":                 ingestSourceFormat(options),
		"experiences_total":             len(events),
		"experiences_persisted":         len(records),
		"experiences_agent_attributed":  agentAttributed,
		"experiences_human_attributed":  humanAttributed,
		"experiences_skipped_uncertain": skippedUncertain,
		"artifact_root":                 ".relia",
		"commit_experiences":            false,
		"experience_shards":             shards,
	}
	if !options.GitHubLive {
		data["input_path"] = sourceRef
	}
	if liveReceipt != nil {
		data["github_live_receipt"] = liveReceipt
	}
	result := passResult("ingest", "ingest", "ingested canonical experience records", start, data)
	result.Warnings = append(result.Warnings, warnings...)
	result.RedactionStatus = "applied"
	result.EvidenceRefs = append(result.EvidenceRefs,
		"docs/product/prd.md#2-ingest",
		"schemas/experience-record.schema.json",
	)
	if !options.GitHubLive {
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "input", Path: sourceRef})
	}
	for _, shard := range shards {
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "experience_shard", Path: shard})
	}
	return result
}

func ingestEventsForOptions(root string, options ingestOptions) ([]map[string]any, string, map[string]any, *CommandError) {
	if options.GitHubLive {
		repo, ingestErr := ingestdoc.ParseGitHubRepoSlug(options.GitHubRepo)
		if ingestErr != nil {
			return nil, "", nil, commandErrorFromIngest(ingestErr)
		}
		token := ""
		if options.GitHubTokenEnv != "" {
			token = os.Getenv(options.GitHubTokenEnv)
		}
		ctx, cancel, client := githubLiveRequestContext()
		defer cancel()
		export, receipt, ingestErr := ingestdoc.FetchGitHubLiveOutcomeExport(ctx, client, ingestdoc.GitHubLiveOptions{
			Repo:                repo,
			PullNumbers:         options.GitHubPulls,
			TokenEnv:            options.GitHubTokenEnv,
			Token:               token,
			TokenScope:          options.GitHubTokenScope,
			NetworkApproved:     options.AllowNetwork,
			CredentialsApproved: options.AllowCredentials,
			HumanApproved:       options.HumanApproved,
			UserAgent:           "relia/" + reliaVersion,
		})
		if ingestErr != nil {
			return nil, "", nil, commandErrorFromIngest(ingestErr)
		}
		content, err := json.Marshal(export)
		if err != nil {
			return nil, "", nil, internalError("could not encode github live outcome export", err)
		}
		sourceRef := "github-live-api"
		if ingestErr := ingestdoc.ValidateJSONRedactionSafe(content, sourceRef); ingestErr != nil {
			return nil, "", nil, commandErrorFromIngest(ingestErr)
		}
		events, ingestErr := ingestdoc.ParseGitHubOutcomeEvents(content, sourceRef)
		if ingestErr != nil {
			return nil, "", nil, commandErrorFromIngest(ingestErr)
		}
		return events, sourceRef, githubLiveReceiptData(receipt), nil
	}

	inputPath := resolveInputPath(root, options.InputPath)
	inputContent, err := os.ReadFile(inputPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", nil, artifactContractError("ingest input is missing", displayPath(root, inputPath))
		}
		return nil, "", nil, internalError("could not read ingest input", err)
	}
	sourceRef := displayPath(root, inputPath)
	events, commandErr := parseIngestEventsForOptions(inputContent, sourceRef, options)
	if commandErr != nil {
		return nil, "", nil, commandErr
	}
	return events, sourceRef, nil, nil
}

func githubLiveRequestContext() (context.Context, context.CancelFunc, *http.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), githubLiveOverallTimeout)
	return ctx, cancel, &http.Client{Timeout: githubLiveHTTPRequestTimeout}
}

func githubLiveReceiptData(receipt ingestdoc.GitHubLiveReceipt) map[string]any {
	return map[string]any{
		"source_format":         receipt.SourceFormat,
		"api_host":              receipt.APIHost,
		"token_env":             receipt.TokenEnv,
		"token_scope":           receipt.TokenScope,
		"read_only":             receipt.ReadOnly,
		"pull_requests_fetched": receipt.PullRequestsFetched,
		"requests_made":         receipt.RequestsMade,
		"pages_fetched":         receipt.PagesFetched,
	}
}
