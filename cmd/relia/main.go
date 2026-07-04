package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
	configdoc "github.com/Clyra-AI/relia/internal/config"
	"github.com/Clyra-AI/relia/internal/diffparse"
	distilldoc "github.com/Clyra-AI/relia/internal/distill"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
	memorydoc "github.com/Clyra-AI/relia/internal/memory"
	resultdoc "github.com/Clyra-AI/relia/internal/result"
	reviewdoc "github.com/Clyra-AI/relia/internal/review"
	"github.com/Clyra-AI/relia/internal/yamlmini"
)

const (
	ExitSuccess = iota
	ExitInternal
	ExitUsage
	ExitOutcomeObservability
	ExitValidation
	ExitGate
	ExitRedactionSafety
	ExitCredential
	ExitDependency
	ExitProvenanceIntegrity
)

const (
	commandSchemaVersion = "1.0"
	reliaVersion         = "0.0.0-dev"
	defaultConfigFile    = configdoc.DefaultFile
)

const (
	badgeStaleAfterDays      = 30
	badgeStaleAfterMergedPRs = 20
	badgeMetadataLastIngest  = "last_ingest_at"
	badgeMetadataMergedPRs   = "merged_prs_since_last_ingest"
)

var requiredCheckFiles = []string{
	"AGENTS.md",
	"WORKFLOW.md",
	"README.md",
	"Makefile",
	".tool-versions",
	"go.mod",
	defaultConfigFile,
	"docs/product/prd.md",
	"docs/dev/dev_guides.md",
	"docs/architecture/architecture_guides.md",
	".github/required-checks.json",
	".github/workflows/validate.yml",
	".github/workflows/codeql.yml",
	".factory/factoryd.example.json",
	".factory/factoryd.autoship.example.json",
}

var requiredSchemaFiles = []string{
	"schemas/experience-record.schema.json",
	"schemas/outcome-evidence.schema.json",
	"schemas/failure-signature.schema.json",
	"schemas/memory-rule.schema.json",
	"schemas/coverage-map.schema.json",
	"schemas/risk-assessment.schema.json",
	"schemas/recurrence-report.schema.json",
	"schemas/compiled-context.schema.json",
	"schemas/command-result.schema.json",
	"schemas/redaction-config.schema.json",
}

var primaryCommands = []string{
	"init",
	"check",
	"ingest",
	"backtest",
	"distill",
	"review",
	"memory",
	"compile",
	"serve",
	"assess",
	"advise",
}

type CommandResult = resultdoc.CommandResult
type Finding = resultdoc.Finding
type CommandError = resultdoc.CommandError
type ArtifactRef = resultdoc.ArtifactRef

type yamlScalar = yamlmini.Scalar
type yamlDocument = yamlmini.Document

type globalFlags struct {
	json    bool
	quiet   bool
	compact bool
	help    bool
	version bool
}

type parsedArgs struct {
	flags       globalFlags
	command     string
	commandArgs []string
}

type ingestOptions = ingestdoc.CLIOptions

type backtestOptions = backtestdoc.Options

type distillOptions = distilldoc.Options

type memoryOptions = memorydoc.Options

type experienceRecord = ingestdoc.Record
type experienceRepo = ingestdoc.Repo
type experienceAttribution = ingestdoc.Attribution
type experienceContext = ingestdoc.Context
type experienceAction = ingestdoc.Action
type experienceOutcome = ingestdoc.Outcome
type experienceSignature = ingestdoc.Signature
type experienceProvenance = ingestdoc.Provenance

type backtestExperience = backtestdoc.Experience
type recurrenceReport = backtestdoc.RecurrenceReport
type recurrenceWindow = backtestdoc.RecurrenceWindow
type recurrenceMetrics = backtestdoc.RecurrenceMetrics
type recurrenceSummary = backtestdoc.RecurrenceSummary
type recurrencePair = backtestdoc.RecurrencePair
type topRepeatedMistake = backtestdoc.TopRepeatedMistake
type backtestFlakeDiscount = backtestdoc.FlakeDiscount
type backtestUncertain = backtestdoc.Uncertain
type baselineComparison = backtestdoc.BaselineComparison
type backtestCitation = backtestdoc.Citation
type reportDiagnostic = backtestdoc.ReportDiagnostic
type reportOperatorFeedback = backtestdoc.OperatorFeedback
type reportBadge = backtestdoc.ReportBadge

type distilledRule = distilldoc.Rule
type distilledRuleProvenance = distilldoc.RuleProvenance
type distilledRuleMetadata = distilldoc.RuleMetadata
type distillCluster = distilldoc.Cluster

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, stdoutIsTerminal(os.Stdout)))
}

func run(args []string, stdout io.Writer, stderr io.Writer, stdoutIsTTY bool) int {
	start := time.Now()
	parsed, parseErr := parseArgs(args)
	if parseErr != nil {
		result := errorResult("relia", "relia", parseErr, start)
		return renderAndExit(stdout, stderr, result, parsed.flags, stdoutIsTTY)
	}

	result := dispatch(parsed, start)
	return renderAndExit(stdout, stderr, result, parsed.flags, stdoutIsTTY)
}

func parseArgs(args []string) (parsedArgs, *CommandError) {
	var parsed parsedArgs
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if parsed.command != "" {
			parsed.commandArgs = append(parsed.commandArgs, arg)
			continue
		}
		switch arg {
		case "--json":
			parsed.flags.json = true
		case "--quiet":
			parsed.flags.quiet = true
		case "--compact":
			parsed.flags.compact = true
		case "--help", "-h":
			parsed.flags.help = true
		case "--version":
			parsed.flags.version = true
		case "--":
			if index+1 >= len(args) {
				return parsed, usageError("missing command after --")
			}
			parsed.command = args[index+1]
			parsed.commandArgs = append(parsed.commandArgs, args[index+2:]...)
			index = len(args)
		default:
			if strings.HasPrefix(arg, "-") {
				return parsed, usageError(fmt.Sprintf("unknown global flag %q", arg))
			}
			parsed.command = arg
		}
	}
	return parsed, nil
}

func dispatch(parsed parsedArgs, start time.Time) CommandResult {
	command := parsed.command
	if parsed.flags.version {
		command = "version"
	}
	if parsed.flags.help || command == "help" {
		return helpResult(start)
	}
	if command == "" {
		command = "status"
	}

	switch command {
	case "status":
		return passResult(command, "status", "relia lifecycle baseline is ready", start, map[string]any{
			"module":              "github.com/Clyra-AI/relia",
			"distribution_target": "standalone_binary",
		})
	case "version":
		return passResult(command, "version", "relia 0.0.0-dev", start, map[string]any{
			"version":        reliaVersion,
			"schema_version": commandSchemaVersion,
		})
	case "init":
		return initResult(parsed.commandArgs, start)
	case "check":
		return checkResult(parsed.commandArgs, start)
	case "ingest":
		return ingestResult(parsed.commandArgs, start)
	case "backtest":
		return backtestResult(parsed.commandArgs, start)
	case "assess":
		return assessResult(parsed.commandArgs, start)
	case "distill":
		return distillResult(parsed.commandArgs, start)
	case "review":
		return reviewResult(parsed.commandArgs, start)
	case "memory":
		return memoryResult(parsed.commandArgs, start)
	case "serve":
		return serveResult(parsed.commandArgs, start)
	case "advise":
		return adviseResult(parsed.commandArgs, start)
	case "models":
		return modelsResult(parsed.commandArgs, start)
	case "compile", "demo", "share":
		return notImplementedResult(command, start)
	default:
		return errorResult(command, command, usageError(fmt.Sprintf("unknown command %q", command)), start)
	}
}

func initResult(args []string, start time.Time) CommandResult {
	if len(args) > 0 {
		return errorResult("init", "init", usageError("init does not accept positional arguments yet"), start)
	}

	wd, err := os.Getwd()
	if err != nil {
		return errorResult("init", "init", internalError("could not inspect working directory", err), start)
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		root = wd
	}
	configPath := filepath.Join(root, defaultConfigFile)
	artifact := ArtifactRef{Kind: "config", Path: defaultConfigFile}
	if _, err := os.Stat(configPath); err == nil {
		if err := configdoc.EnsureArtifactSkeleton(root); err != nil {
			return errorResult("init", "init", internalError("could not write artifact skeleton", err), start)
		}
		if err := configdoc.EnsureReliaGitIgnore(root); err != nil {
			return errorResult("init", "init", internalError("could not update .gitignore", err), start)
		}
		artifactSkeletonDirs := configdoc.ArtifactSkeletonPaths()
		result := passResult("init", "init", "relia.yaml already exists", start, map[string]any{
			"config_path":             defaultConfigFile,
			"created":                 false,
			"artifact_skeleton_paths": artifactSkeletonDirs,
		})
		result.Artifacts = append(result.Artifacts, artifact)
		for _, dir := range artifactSkeletonDirs {
			result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "artifact_directory", Path: dir})
		}
		return result
	} else if !errors.Is(err, os.ErrNotExist) {
		return errorResult("init", "init", internalError("could not inspect relia.yaml", err), start)
	}

	if err := os.WriteFile(configPath, []byte(defaultConfigYAML()), 0o644); err != nil {
		return errorResult("init", "init", internalError("could not write relia.yaml", err), start)
	}
	if err := configdoc.EnsureArtifactSkeleton(root); err != nil {
		return errorResult("init", "init", internalError("could not write artifact skeleton", err), start)
	}
	if err := configdoc.EnsureReliaGitIgnore(root); err != nil {
		return errorResult("init", "init", internalError("could not update .gitignore", err), start)
	}
	artifactSkeletonDirs := configdoc.ArtifactSkeletonPaths()
	result := passResult("init", "init", "created relia.yaml", start, map[string]any{
		"config_path":             defaultConfigFile,
		"created":                 true,
		"artifact_skeleton_paths": artifactSkeletonDirs,
	})
	result.Artifacts = append(result.Artifacts, artifact)
	for _, dir := range artifactSkeletonDirs {
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "artifact_directory", Path: dir})
	}
	return result
}

func checkResult(args []string, start time.Time) CommandResult {
	if len(args) > 0 {
		return errorResult("check", "check", usageError("check does not accept positional arguments yet"), start)
	}

	wd, err := os.Getwd()
	if err != nil {
		return errorResult("check", "check", internalError("could not inspect working directory", err), start)
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return errorResult("check", "check", configError("could not locate repository root from current directory"), start)
	}

	var missing []string
	for _, rel := range requiredCheckFiles {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				missing = append(missing, rel)
				continue
			}
			return errorResult("check", "check", internalError("could not inspect "+rel, err), start)
		}
	}
	if len(missing) > 0 {
		return errorResult("check", "check", validationError("required local operating-pack files are missing", missing), start)
	}

	warnings, commandErr := validateReliaConfig(root)
	if commandErr != nil {
		return errorResult("check", "check", commandErr, start)
	}
	if commandErr := validateSchemaContracts(root); commandErr != nil {
		return errorResult("check", "check", commandErr, start)
	}
	if commandErr := validateMemoryRuleArtifacts(root); commandErr != nil {
		return errorResult("check", "check", commandErr, start)
	}

	result := passResult("check", "check", "local operating pack baseline is present", start, map[string]any{
		"checked_paths":        len(requiredCheckFiles),
		"repo_root":            ".",
		"schema_contracts":     len(requiredSchemaFiles),
		"privacy_default":      "local_only",
		"artifact_schema_refs": requiredSchemaFiles,
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "config", Path: defaultConfigFile})
	for _, schemaFile := range requiredSchemaFiles {
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "schema", Path: schemaFile})
	}
	return result
}

func loadBacktestExperiences(root string) ([]backtestExperience, []string, string, *CommandError) {
	pattern := filepath.Join(root, ".relia", "experiences", "*.jsonl")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, "", internalError("could not inspect experience shards", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, nil, "", artifactContractError("backtest found no experience shards under .relia/experiences", ".relia/experiences")
	}

	var records []backtestExperience
	sourceArtifacts := make([]string, 0, len(paths))
	digestParts := make([]string, 0, len(paths))
	for _, path := range paths {
		rel := displayPath(root, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, "", internalError("could not read experience shard "+rel, err)
		}
		sourceArtifacts = append(sourceArtifacts, rel)
		shardDigestLines := []string{}
		for lineNumber, line := range strings.Split(string(content), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			ref := fmt.Sprintf("%s:%d", rel, lineNumber+1)
			var event map[string]any
			if err := decodeJSONUseNumber(line, &event); err != nil {
				return nil, nil, "", artifactContractError(fmt.Sprintf("experience shard line %d is not valid JSON", lineNumber+1), fmt.Sprintf("%s:%d", rel, lineNumber+1))
			}
			shardDigestLines = append(shardDigestLines, experienceDigestLine(event))
			if ingestErr := ingestdoc.ValidateEventMemorySource(event, ref); ingestErr != nil {
				return nil, nil, "", commandErrorFromIngest(ingestErr)
			}
			content, err := json.Marshal(event)
			if err != nil {
				return nil, nil, "", internalError("could not decode experience shard line", err)
			}
			var record experienceRecord
			if err := decodeJSONUseNumber(string(content), &record); err != nil {
				return nil, nil, "", artifactContractError(fmt.Sprintf("experience shard line %d is not valid JSON", lineNumber+1), ref)
			}
			recordedAt, commandErr := validateBacktestExperience(record, ref)
			if commandErr != nil {
				return nil, nil, "", commandErr
			}
			records = append(records, backtestExperience{
				Record:     record,
				RecordedAt: recordedAt,
				SourcePath: rel,
				SourceLine: lineNumber + 1,
			})
		}
		digestParts = append(digestParts, rel+"\x00"+sha256String(strings.Join(shardDigestLines, "\n")))
	}
	if len(records) == 0 {
		return nil, nil, "", artifactContractError("backtest found no experience records in .relia/experiences", ".relia/experiences")
	}
	sort.Strings(digestParts)
	return records, sourceArtifacts, sha256String(strings.Join(digestParts, "\x00")), nil
}

func experienceDigestLine(event map[string]any) string {
	normalized, _ := cloneJSONValue(event).(map[string]any)
	if normalized == nil {
		normalized = map[string]any{}
	}
	if metadata, ok := normalized["metadata"].(map[string]any); ok {
		delete(metadata, badgeMetadataLastIngest)
		delete(metadata, badgeMetadataMergedPRs)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		clone := make(map[string]any, len(typed))
		for key, item := range typed {
			clone[key] = cloneJSONValue(item)
		}
		return clone
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneJSONValue(item)
		}
		return clone
	default:
		return typed
	}
}

func loadDistillExperiences(root string, config yamlDocument, options distillOptions) ([]backtestExperience, []string, string, *CommandError) {
	if strings.TrimSpace(options.InputPath) == "" {
		return loadBacktestExperiences(root)
	}
	inputPath := resolveInputPath(root, options.InputPath)
	rel := displayPath(root, inputPath)
	content, err := os.ReadFile(inputPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, "", artifactContractError("distill input is missing", rel)
		}
		return nil, nil, "", internalError("could not read distill input", err)
	}
	events, commandErr := parseIngestEvents(content, rel)
	if commandErr != nil {
		return nil, nil, "", commandErr
	}
	records := make([]backtestExperience, 0, len(events))
	for index, event := range events {
		eventRef := fmt.Sprintf("%s:%d", rel, index+1)
		redacted, commandErr := redactForPersistence(event, eventRef)
		if commandErr != nil {
			return nil, nil, "", commandErr
		}
		redactedEvent, ok := redacted.(map[string]any)
		if !ok {
			return nil, nil, "", artifactContractError("distill input event must be a JSON object", eventRef)
		}
		if record, canonical, commandErr := canonicalDistillInputExperienceRecord(redactedEvent, eventRef); commandErr != nil {
			return nil, nil, "", commandErr
		} else if canonical {
			recordedAt, commandErr := validateBacktestExperience(record, eventRef)
			if commandErr != nil {
				return nil, nil, "", commandErr
			}
			records = append(records, backtestExperience{
				Record:     record,
				RecordedAt: recordedAt,
				SourcePath: rel,
				SourceLine: index + 1,
			})
			continue
		}
		record, skipped, commandErr := normalizeExperienceRecord(config, redactedEvent, index, eventRef)
		if commandErr != nil {
			return nil, nil, "", commandErr
		}
		if skipped {
			continue
		}
		recordedAt, commandErr := validateBacktestExperience(record, eventRef)
		if commandErr != nil {
			return nil, nil, "", commandErr
		}
		records = append(records, backtestExperience{
			Record:     record,
			RecordedAt: recordedAt,
			SourcePath: rel,
			SourceLine: index + 1,
		})
	}
	if len(records) == 0 {
		return nil, nil, "", artifactContractError("distill found no usable experience records in input", rel)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].RecordedAt.Equal(records[j].RecordedAt) {
			return records[i].Record.ExperienceID < records[j].Record.ExperienceID
		}
		return records[i].RecordedAt.Before(records[j].RecordedAt)
	})
	digest := sha256String(rel + "\x00" + sha256String(string(content)))
	return records, []string{rel}, digest, nil
}

func canonicalDistillInputExperienceRecord(event map[string]any, ref string) (experienceRecord, bool, *CommandError) {
	record, canonical, ingestErr := ingestdoc.CanonicalDistillInputRecord(event, ref)
	return record, canonical, commandErrorFromIngest(ingestErr)
}

func validateBacktestExperience(record experienceRecord, ref string) (time.Time, *CommandError) {
	recordedAt, ingestErr := ingestdoc.ValidateRecord(record, ref, commandSchemaVersion)
	return recordedAt, commandErrorFromIngest(ingestErr)
}

func buildRecurrenceReport(root string, config yamlDocument, records []backtestExperience, sourceArtifacts []string, sourceDigest string, options backtestOptions, windowDays int) (recurrenceReport, *CommandError) {
	backtestdoc.SortExperiences(records)
	window, windowRecords := backtestdoc.WindowRecords(records, windowDays)
	flakeHeuristics := backtestdoc.AutomaticFlakeDiscounts(windowRecords)
	priorBySignature := map[string][]backtestExperience{}
	confirmed := []recurrencePair{}
	possible := []recurrencePair{}
	flakes := []backtestFlakeDiscount{}
	uncertain := []backtestUncertain{}
	citationMap := map[int]backtestCitation{}
	prsAnalyzed := map[int]bool{}
	agentAttributedPRs := map[int]bool{}
	agentAttributedExperiences := 0
	agentFailuresByOutcomeKind := map[string]int{}
	summary := recurrenceSummary{
		ExperienceCount:       len(records),
		WindowExperienceCount: len(windowRecords),
	}
	for _, current := range windowRecords {
		record := current.Record
		prsAnalyzed[record.Action.PR] = true
		if record.Attribution.ActorKind == "agent" {
			agentAttributedPRs[record.Action.PR] = true
			agentAttributedExperiences++
			if backtestdoc.IsFailureOutcome(record.Outcome.Kind) {
				agentFailuresByOutcomeKind[record.Outcome.Kind]++
			}
		}
		if record.Attribution.ActorKind == "uncertain" {
			summary.AttributionUncertainCount++
			uncertain = append(uncertain, backtestUncertain{
				ExperienceID:          record.ExperienceID,
				PR:                    record.Action.PR,
				OutcomeKind:           record.Outcome.Kind,
				AttributionMethod:     record.Attribution.Method,
				AttributionConfidence: record.Attribution.Confidence,
				ExcludedFromERR:       true,
				Ref:                   backtestdoc.SourceLineRef(current),
				Reason:                "Excluded from headline ERR because attribution was ambiguous and the default policy is uncertain: exclude.",
			})
			backtestdoc.AddCitation(citationMap, current)
			continue
		}
		if !backtestdoc.IsFailureOutcome(record.Outcome.Kind) {
			summary.NonFailureOutcomeCount++
			continue
		}
		signatureKeys := backtestdoc.RecurrenceSignatureKeys(record)
		if record.Attribution.ActorKind != "agent" {
			summary.HumanFailureExcludedCount++
			if record.FlakeDiscount > 0 {
				backtestdoc.AddCitation(citationMap, current)
				continue
			}
			backtestdoc.AppendRecurrencePrior(priorBySignature, signatureKeys, current)
			continue
		}
		summary.AgentFailureDenominator++
		if backtestdoc.IsFlakeDiscounted(current, flakeHeuristics) {
			summary.FlakeDiscountedCount++
			flakes = append(flakes, backtestdoc.BuildFlakeDiscount(current, windowRecords, flakeHeuristics))
			backtestdoc.AddCitation(citationMap, current)
			continue
		}
		if priors := backtestdoc.RecurrencePriorCandidates(priorBySignature, signatureKeys); len(priors) > 0 {
			prior, confidence, ok := backtestdoc.SelectRecurrencePrior(priors, current)
			if !ok {
				backtestdoc.AppendRecurrencePrior(priorBySignature, signatureKeys, current)
				continue
			}
			pair := backtestdoc.BuildRecurrencePair(prior, current)
			if confidence == "confirmed" {
				pair.Confidence = "confirmed"
				pair.Reason = "Exact reliable signature repeated with overlapping paths."
				confirmed = append(confirmed, pair)
			} else {
				pair.Confidence = "possible"
				pair.Reason = "Exact signature repeated, but extraction confidence or path overlap was insufficient; excluded from the headline ERR."
				possible = append(possible, pair)
			}
			backtestdoc.AddCitation(citationMap, prior)
			backtestdoc.AddCitation(citationMap, current)
		}
		backtestdoc.AppendRecurrencePrior(priorBySignature, signatureKeys, current)
	}
	backtestdoc.SortRecurrencePairs(confirmed)
	backtestdoc.SortRecurrencePairs(possible)
	backtestdoc.SortFlakeDiscounts(flakes)
	backtestdoc.SortUncertain(uncertain)
	citations := backtestdoc.Citations(citationMap)
	summary.ConfirmedRecurrenceCount = len(confirmed)
	summary.PossibleRecurrenceCount = len(possible)
	if summary.AgentFailureDenominator > 0 {
		summary.HeadlineERR = roundFloat(float64(summary.ConfirmedRecurrenceCount)/float64(summary.AgentFailureDenominator), 4)
	}
	summary.HeadlineERRPercent = fmt.Sprintf("%.1f%%", summary.HeadlineERR*100)
	metrics := backtestdoc.BuildReportMetrics(summary, prsAnalyzed, agentAttributedPRs, agentAttributedExperiences, agentFailuresByOutcomeKind)
	baseline, commandErr := compareBacktestBaseline(root, options.BaselinePath, summary.HeadlineERR, sourceDigest, window)
	if commandErr != nil {
		return recurrenceReport{}, commandErr
	}
	gate := backtestdoc.BuildGate(config, summary.HeadlineERR)
	metadata := backtestdoc.BuildReportMetadata(records, windowRecords, windowDays, sourceDigest)
	report := recurrenceReport{
		ObjectType:           "relia.recurrence_report",
		SchemaVersion:        commandSchemaVersion,
		SourceArtifacts:      append([]string(nil), sourceArtifacts...),
		Window:               window,
		Summary:              summary,
		Metrics:              metrics,
		HeadlineERR:          summary.HeadlineERR,
		ConfirmedRecurrences: confirmed,
		PossibleRecurrences:  possible,
		TopRepeatedMistakes:  backtestdoc.BuildTopRepeatedMistakes(confirmed),
		FlakeDiscounts:       flakes,
		AttributionUncertain: uncertain,
		Baseline:             baseline,
		Gate:                 gate,
		Citations:            citations,
		Diagnostics:          backtestdoc.BuildReportDiagnostics(summary, baseline, sourceArtifacts),
		OperatorFeedback:     backtestdoc.BuildReportOperatorFeedback(summary),
		Metadata:             metadata,
	}
	report.ReportID = backtestdoc.BuildReportID(report.Window, sourceDigest, summary)
	report.Badge = backtestdoc.BuildReportBadge(report)
	return report, nil
}

func roundFloat(value float64, places int) float64 {
	if places < 0 {
		return value
	}
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}

func compareBacktestBaseline(root string, baselinePath string, headlineERR float64, sourceDigest string, window recurrenceWindow) (baselineComparison, *CommandError) {
	clean, ok := configdoc.CleanRepoPath(baselinePath)
	if !ok {
		return baselineComparison{}, usageError("backtest baseline path must be repo-relative")
	}
	rel := filepath.ToSlash(clean)
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return backtestdoc.MissingBaselineComparison(rel), nil
		}
		return baselineComparison{}, internalError("could not read ERR baseline", err)
	}
	baseline, err := backtestdoc.CompareBaselineJSON(content, rel, headlineERR, sourceDigest, window)
	if errors.Is(err, backtestdoc.ErrInvalidBaselineJSON) {
		return baselineComparison{}, artifactContractError("ERR baseline is not valid JSON", rel)
	}
	if errors.Is(err, backtestdoc.ErrInvalidBaselineHeadlineERR) {
		return baselineComparison{}, artifactContractError("ERR baseline missing numeric headline_err", rel)
	}
	if err != nil {
		return baselineComparison{}, internalError("could not compare ERR baseline", err)
	}
	return baseline, nil
}

func writeBacktestReports(root string, report recurrenceReport, reportDir string) (string, string, *CommandError) {
	cleanReportDir, ok := configdoc.CleanRepoPath(reportDir)
	if !ok {
		return "", "", usageError("backtest report directory must be repo-relative")
	}
	reportDirPath := filepath.Join(root, filepath.FromSlash(filepath.ToSlash(cleanReportDir)))
	if err := os.MkdirAll(reportDirPath, 0o755); err != nil {
		return "", "", internalError("could not create backtest report directory", err)
	}
	jsonRel := filepath.ToSlash(filepath.Join(filepath.ToSlash(cleanReportDir), report.ReportID+".json"))
	htmlRel := filepath.ToSlash(filepath.Join(filepath.ToSlash(cleanReportDir), report.ReportID+".html"))
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", "", internalError("could not encode recurrence report", err)
	}
	encoded = append(encoded, '\n')
	jsonPath := filepath.Join(root, filepath.FromSlash(jsonRel))
	htmlPath := filepath.Join(root, filepath.FromSlash(htmlRel))
	if commandErr := ensureReplaceableBacktestReportPath(jsonPath, "JSON"); commandErr != nil {
		return "", "", commandErr
	}
	if commandErr := ensureReplaceableBacktestReportPath(htmlPath, "HTML"); commandErr != nil {
		return "", "", commandErr
	}
	jsonTempPath, commandErr := writeBacktestReportTemp(reportDirPath, jsonPath, encoded, "JSON")
	if commandErr != nil {
		return "", "", commandErr
	}
	defer func() {
		if jsonTempPath != "" {
			_ = os.Remove(jsonTempPath)
		}
	}()
	htmlTempPath, commandErr := writeBacktestReportTemp(reportDirPath, htmlPath, []byte(backtestdoc.RenderHTML(report)), "HTML")
	if commandErr != nil {
		return "", "", commandErr
	}
	defer func() {
		if htmlTempPath != "" {
			_ = os.Remove(htmlTempPath)
		}
	}()
	if err := os.Rename(htmlTempPath, htmlPath); err != nil {
		return "", "", internalError("could not write HTML recurrence report", err)
	}
	htmlTempPath = ""
	if err := os.Rename(jsonTempPath, jsonPath); err != nil {
		return "", "", internalError("could not write JSON recurrence report", err)
	}
	jsonTempPath = ""
	return jsonRel, htmlRel, nil
}

func ensureReplaceableBacktestReportPath(path string, format string) *CommandError {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return internalError("could not write "+format+" recurrence report", fmt.Errorf("%s is a directory", path))
		}
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return internalError("could not write "+format+" recurrence report", err)
}

func writeBacktestReportTemp(dir string, finalPath string, content []byte, format string) (string, *CommandError) {
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(finalPath)+".tmp-*")
	if err != nil {
		return "", internalError("could not write "+format+" recurrence report", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return "", internalError("could not write "+format+" recurrence report", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", internalError("could not write "+format+" recurrence report", err)
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return "", internalError("could not write "+format+" recurrence report", err)
	}
	cleanup = false
	return tempPath, nil
}

type backtestBaselineSnapshot struct {
	Path    string
	Content []byte
	Exists  bool
}

func snapshotBacktestBaseline(root string, baselinePath string) (backtestBaselineSnapshot, *CommandError) {
	clean, ok := configdoc.CleanRepoPath(baselinePath)
	if !ok {
		return backtestBaselineSnapshot{}, usageError("backtest baseline path must be repo-relative")
	}
	path := filepath.Join(root, filepath.FromSlash(filepath.ToSlash(clean)))
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return backtestBaselineSnapshot{Path: path, Exists: false}, nil
		}
		return backtestBaselineSnapshot{}, internalError("could not snapshot ERR baseline", err)
	}
	return backtestBaselineSnapshot{Path: path, Content: append([]byte(nil), content...), Exists: true}, nil
}

func restoreBacktestBaseline(snapshot backtestBaselineSnapshot) *CommandError {
	if snapshot.Path == "" {
		return nil
	}
	if snapshot.Exists {
		if err := os.MkdirAll(filepath.Dir(snapshot.Path), 0o755); err != nil {
			return internalError("could not restore ERR baseline directory", err)
		}
		if err := os.WriteFile(snapshot.Path, snapshot.Content, 0o644); err != nil {
			return internalError("could not restore ERR baseline", err)
		}
		return nil
	}
	if err := os.Remove(snapshot.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return internalError("could not remove rolled back ERR baseline", err)
	}
	return nil
}

func saveBacktestBaseline(root string, report recurrenceReport, baselinePath string) *CommandError {
	clean, ok := configdoc.CleanRepoPath(baselinePath)
	if !ok {
		return usageError("backtest baseline path must be repo-relative")
	}
	rel := filepath.ToSlash(clean)
	payload := map[string]any{
		"object_type":      "relia.err_baseline",
		"schema_version":   commandSchemaVersion,
		"baseline_id":      "baseline_" + shortHash(report.ReportID),
		"report_id":        report.ReportID,
		"headline_err":     report.HeadlineERR,
		"window":           report.Window,
		"source_artifacts": report.SourceArtifacts,
		"metadata": map[string]any{
			"source_artifact_digest": report.Metadata["source_artifact_digest"],
			"created_by":             "relia backtest --save-baseline",
		},
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return internalError("could not encode ERR baseline", err)
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return internalError("could not create ERR baseline directory", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return internalError("could not write ERR baseline", err)
	}
	return nil
}

func savedBacktestBaselineComparison(baselinePath string, headlineERR float64) (baselineComparison, *CommandError) {
	clean, ok := configdoc.CleanRepoPath(baselinePath)
	if !ok {
		return baselineComparison{}, usageError("backtest baseline path must be repo-relative")
	}
	return backtestdoc.SavedBaselineComparison(filepath.ToSlash(clean), headlineERR), nil
}

func distillInputPathMetadata(options distillOptions, sourceArtifacts []string) string {
	if strings.TrimSpace(options.InputPath) == "" {
		return ""
	}
	if len(sourceArtifacts) == 0 {
		return ""
	}
	return sourceArtifacts[0]
}

func buildDistilledRules(root string, config yamlDocument, records []backtestExperience, sourceArtifacts []string, sourceDigest string, embeddingMode string, options distillOptions) ([]distilledRule, *CommandError) {
	if len(records) == 0 {
		return nil, artifactContractError("distill found no experience records in .relia/experiences", ".relia/experiences")
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].RecordedAt.Equal(records[j].RecordedAt) {
			return records[i].Record.ExperienceID < records[j].Record.ExperienceID
		}
		return records[i].RecordedAt.Before(records[j].RecordedAt)
	})
	anchor := records[len(records)-1].RecordedAt.UTC()
	clusters := distilldoc.BuildClusters(records)
	flakeHeuristics := backtestdoc.AutomaticFlakeDiscounts(records)
	provider, _ := distilldoc.Provider(config)
	reviewRequired := distilldoc.ReviewRequired(config)

	var rules []distilledRule
	for _, cluster := range clusters {
		failures := distilldoc.FailureEvidence(cluster.Records)
		positives := distilldoc.PositiveEvidence(cluster.Records)
		if len(failures) > 0 && !distilldoc.AllEvidenceDiscounted(failures, flakeHeuristics) {
			rule, ok := buildDistilledRule(root, "avoid", cluster, failures, distilldoc.AvoidContradictions(failures, positives), anchor, sourceArtifacts, sourceDigest, provider, embeddingMode, reviewRequired, options, flakeHeuristics)
			if ok {
				rules = append(rules, rule)
			}
		}
		held := distilldoc.HeldEvidence(cluster.Records)
		if len(held) > 0 {
			playbookEvidence := positives
			if len(playbookEvidence) == 0 {
				playbookEvidence = held
			}
			contradictions := distilldoc.PlaybookContradictions(playbookEvidence, failures)
			rule, ok := buildDistilledRule(root, "playbook", cluster, playbookEvidence, contradictions, anchor, sourceArtifacts, sourceDigest, provider, embeddingMode, reviewRequired, options, flakeHeuristics)
			if ok {
				rules = append(rules, rule)
			}
		}
	}
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].ID < rules[j].ID
	})
	return rules, nil
}

func distillClusterKey(record experienceRecord) string {
	return distilldoc.ClusterKey(record)
}

func distillClusterProvenance(embeddingMode string) string {
	switch embeddingMode {
	case "signature":
		return "signature_only"
	case "local":
		return "local_manifest_verified"
	case "provider":
		return "provider_opt_in"
	default:
		return "unknown"
	}
}

func buildDistilledRule(root string, kind string, cluster distillCluster, evidence []backtestExperience, contradictions int, anchor time.Time, sourceArtifacts []string, sourceDigest string, provider string, embeddingMode string, reviewRequired bool, options distillOptions, flakeHeuristics map[string]string) (distilledRule, bool) {
	scopePaths := distilldoc.ScopePaths(evidence)
	scopeSignals := distilldoc.ScopeSignals(cluster, evidence)
	if len(scopePaths) == 0 && len(scopeSignals) == 0 {
		return distilledRule{}, false
	}
	confidence, metadata := distilldoc.ConfidenceMetadata(evidence, contradictions, anchor, options.HalfLifeDays, flakeHeuristics)
	status, reason := distilledRuleStatus(root, kind, scopePaths, len(evidence), contradictions, reviewRequired)
	metadata.LifecycleReason = reason
	metadata.ClusterKey = strings.ReplaceAll(cluster.Key, "\x00", "|")
	metadata.ClusterKeyHash = shortHash(cluster.Key)
	metadata.ClusterProvenance = distillClusterProvenance(embeddingMode)
	metadata.SourceArtifacts = append([]string(nil), sourceArtifacts...)
	metadata.SourceArtifactDigest = sourceDigest
	metadata.Provider = provider
	metadata.EmbeddingMode = embeddingMode
	metadata.ReviewRequired = reviewRequired
	metadata.DeterministicFallback = provider == "none" && embeddingMode == "signature"
	metadata.MemorySource = "verified_outcome_events"
	metadata.SourceRecordType = "relia.experience_record"
	metadata.ExcludedMemorySources = []string{"agent_self_report", "agent_reflection"}
	id := distilldoc.RuleID(kind, cluster)
	return distilledRule{
		ID:              id,
		Kind:            kind,
		Status:          status,
		Statement:       distilldoc.RuleStatement(kind, cluster, scopePaths),
		ScopePaths:      scopePaths,
		ScopeSignals:    scopeSignals,
		Confidence:      confidence,
		EvidenceCount:   len(evidence),
		Contradictions:  contradictions,
		Experiences:     distilldoc.RuleExperienceIDs(evidence),
		Provenance:      distilldoc.RuleProvenanceRefs(evidence),
		ReviewLabel:     distilldoc.ReviewLabel(status, confidence),
		StatementOrigin: "cluster_summary",
		Metadata:        metadata,
	}, true
}

func distilledRuleStatus(root string, kind string, scopePaths []string, evidenceCount int, contradictions int, reviewRequired bool) (string, string) {
	if len(scopePaths) > 0 && allScopePathsMissing(root, scopePaths) {
		return "stale", "all scoped paths are missing from the working tree"
	}
	if contradictions > 0 {
		if kind == "playbook" {
			return "contradicted", "later failure evidence contradicts the drafted playbook rule"
		}
		return "contradicted", "later held or clean evidence contradicts the drafted avoid rule"
	}
	if !reviewRequired {
		return "candidate", "review_required=false is surfaced but human review is still required before activation"
	}
	return "candidate", "human review required before activation"
}

func allScopePathsMissing(root string, scopePaths []string) bool {
	for _, scopePath := range scopePaths {
		clean, ok := configdoc.CleanRepoPath(scopePath)
		if !ok {
			continue
		}
		if configdoc.WorkingTreePathMatches(root, clean) {
			return false
		}
	}
	return true
}

func writeDistilledRules(root string, ruleDir string, rules []distilledRule) ([]ArtifactRef, []distilledRule, *CommandError) {
	cleanRuleDir, ok := configdoc.CleanRepoPath(ruleDir)
	if !ok {
		return nil, nil, usageError("distill rule directory must be repo-relative")
	}
	ruleDirRel := filepath.ToSlash(cleanRuleDir)
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(ruleDirRel)), 0o755); err != nil {
		return nil, nil, internalError("could not create memory rule directory", err)
	}
	artifacts := make([]ArtifactRef, 0, len(rules))
	for index, rule := range rules {
		rule = mergeExistingRuleLifecycle(root, ruleDirRel, rule)
		rules[index] = rule
		rel := filepath.ToSlash(filepath.Join(ruleDirRel, rule.ID+".yaml"))
		path := filepath.Join(root, filepath.FromSlash(rel))
		content := []byte(distilldoc.RenderRuleYAML(rule))
		if commandErr := writeAtomicRepoFile(path, content, "memory rule"); commandErr != nil {
			return nil, nil, commandErr
		}
		artifacts = append(artifacts, ArtifactRef{Kind: "memory_rule", Path: rel})
	}
	return artifacts, rules, nil
}

func mergeExistingRuleLifecycle(root string, ruleDir string, rule distilledRule) distilledRule {
	path, commandErr := reviewdoc.FindRulePath(root, ruleDir, rule.ID, reviewUpdateOptions())
	if commandErr != nil {
		return rule
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return rule
	}
	document, err := parseYAMLDocument(string(content))
	if err != nil {
		return rule
	}
	return preserveExistingAcceptedRuleLifecycle(rule, document)
}

func yamlFloat(value float64) string {
	return distilldoc.YAMLFloat(value)
}

func parseUnifiedDiffTouchedPaths(content []byte, ref string) ([]string, *CommandError) {
	paths, err := diffparse.TouchedPaths(content)
	if err == nil {
		return paths, nil
	}
	if errors.Is(err, diffparse.ErrNoRepoRelativePaths) {
		return nil, artifactContractError("assess input diff contains no repo-relative paths", ref)
	}
	return nil, internalError("could not parse assess input diff", err)
}
