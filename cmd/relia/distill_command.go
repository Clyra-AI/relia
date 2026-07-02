package main

import (
	"os"
	"strings"
	"time"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	distilldoc "github.com/Clyra-AI/relia/internal/distill"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

func distillResult(args []string, start time.Time) CommandResult {
	options, parseErr := distilldoc.ParseArgs(args)
	withFormat := func(result CommandResult) CommandResult {
		if options.Format == "json" {
			result.MachineReadable = true
		}
		return result
	}
	if parseErr != nil {
		return withFormat(errorResult("distill", "distill", usageError(parseErr.Message), start))
	}
	wd, err := os.Getwd()
	if err != nil {
		return withFormat(errorResult("distill", "distill", internalError("could not inspect working directory", err), start))
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return withFormat(errorResult("distill", "distill", configError("could not locate repository root from current directory"), start))
	}
	warnings, commandErr := validateReliaConfigForDistill(root, options.Embeddings)
	if commandErr != nil {
		return withFormat(errorResult("distill", "distill", commandErr, start))
	}
	config, commandErr := readReliaConfig(root)
	if commandErr != nil {
		return withFormat(errorResult("distill", "distill", commandErr, start))
	}
	providerConfig, configErr := configdoc.ProviderConfigFromYAML(config)
	if configErr != nil {
		return withFormat(errorResult("distill", "distill", commandErrorFromConfig(configErr), start))
	}
	embeddingMode := distilldoc.EffectiveEmbeddingMode(config, options.Embeddings)
	records, sourceArtifacts, sourceDigest, commandErr := loadDistillExperiences(root, config, options)
	if commandErr != nil {
		return withFormat(errorResult("distill", "distill", commandErr, start))
	}
	if providerConfig.Provider != "none" || embeddingMode == "provider" {
		providerRecords := make([]ingestdoc.Record, 0, len(records))
		for _, record := range records {
			providerRecords = append(providerRecords, record.Record)
		}
		providerPlan, configErr := distilldoc.BuildProviderPlan(providerConfig, providerRecords, embeddingMode, sourceArtifacts, sourceDigest)
		if configErr != nil {
			return withFormat(errorResultWithData("distill", "distill", commandErrorFromConfig(configErr), start, map[string]any{
				"provider_plan": providerPlan,
			}))
		}
		cost := providerPlan["cost"].(distilldoc.CostEstimate)
		if cost.CapStatus == "exceeded" {
			return withFormat(errorResultWithData("distill", "distill", dependencyError("provider-backed distill estimated cost exceeds distill.max_cost_usd_per_run; no provider call was attempted", providerConfig.ProviderRef), start, map[string]any{
				"provider_plan": providerPlan,
			}))
		}
		return withFormat(errorResultWithData("distill", "distill", dependencyError("provider-backed distill requires an approved model_provider_endpoint gate; no experience records were sent", providerConfig.ProviderRef), start, map[string]any{
			"provider_plan": providerPlan,
		}))
	}
	rules, commandErr := buildDistilledRules(root, config, records, sourceArtifacts, sourceDigest, embeddingMode, options)
	if commandErr != nil {
		return withFormat(errorResult("distill", "distill", commandErr, start))
	}
	ruleArtifacts, commandErr := writeDistilledRules(root, options.RuleDir, rules)
	if commandErr != nil {
		return withFormat(errorResult("distill", "distill", commandErr, start))
	}
	if commandErr := validateMemoryRuleArtifacts(root); commandErr != nil {
		return withFormat(errorResult("distill", "distill", commandErr, start))
	}

	statusCounts := distilldoc.StatusCounts(rules)
	ruleArtifactPaths := make([]string, 0, len(ruleArtifacts))
	for _, artifact := range ruleArtifacts {
		ruleArtifactPaths = append(ruleArtifactPaths, artifact.Path)
	}
	result := passResult("distill", "distill", "drafted deterministic memory rules from local experience records", start, map[string]any{
		"format":                     options.Format,
		"input_path":                 distillInputPathMetadata(options, sourceArtifacts),
		"rule_dir":                   options.RuleDir,
		"rules_written":              len(rules),
		"candidate_rules":            statusCounts["candidate"],
		"active_rules":               statusCounts["active"],
		"stale_rules":                statusCounts["stale"],
		"contradicted_rules":         statusCounts["contradicted"],
		"retired_rules":              statusCounts["retired"],
		"provider":                   providerConfig.Provider,
		"embedding_mode":             embeddingMode,
		"review_required":            distilldoc.ReviewRequired(config),
		"deterministic_fallback":     providerConfig.Provider == "none" && embeddingMode == "signature",
		"confidence_model":           "evidence_count+recency_half_life+contradictions+flake_discount+extraction_confidence",
		"drafting_model_confidence":  0,
		"provider_cost":              distilldoc.NoProviderCost(),
		"decay_half_life_days":       options.HalfLifeDays,
		"source_artifacts":           sourceArtifacts,
		"source_artifact_digest":     sourceDigest,
		"drafted_rules":              distilldoc.DraftedRuleData(rules, ruleArtifactPaths),
		"provider_data_disclosure":   "none; provider is none and no network call was attempted",
		"redacted_records_only":      true,
		"local_privacy_default":      true,
		"review_gate_disabled_label": "distill.review_required=false is surfaced but does not auto-accept drafted rules in the MVP",
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs,
		"schemas/memory-rule.schema.json",
		"docs/product/prd.md#distill-calibrate-review-memory-page",
	)
	sourceArtifactKind := "experience_shard"
	if strings.TrimSpace(options.InputPath) != "" {
		sourceArtifactKind = "input"
	}
	for _, artifact := range sourceArtifacts {
		result.EvidenceRefs = append(result.EvidenceRefs, artifact)
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: sourceArtifactKind, Path: artifact})
	}
	for _, artifact := range ruleArtifacts {
		result.EvidenceRefs = append(result.EvidenceRefs, artifact.Path)
		result.Artifacts = append(result.Artifacts, artifact)
	}
	result.RedactionStatus = "applied"
	return withFormat(result)
}
