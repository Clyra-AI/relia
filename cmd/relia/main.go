package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	advisedoc "github.com/Clyra-AI/relia/internal/advise"
	assessdoc "github.com/Clyra-AI/relia/internal/assess"
	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
	configdoc "github.com/Clyra-AI/relia/internal/config"
	"github.com/Clyra-AI/relia/internal/diffparse"
	distilldoc "github.com/Clyra-AI/relia/internal/distill"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
	memorydoc "github.com/Clyra-AI/relia/internal/memory"
	modelpulldoc "github.com/Clyra-AI/relia/internal/modelpull"
	resultdoc "github.com/Clyra-AI/relia/internal/result"
	reviewdoc "github.com/Clyra-AI/relia/internal/review"
	servedoc "github.com/Clyra-AI/relia/internal/serve"
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

type assessOptions = assessdoc.CLIOptions

type distillOptions = distilldoc.Options

type serveOptions = servedoc.Options

type adviseOptions struct {
	InputPath   string
	Format      string
	StatePath   string
	CommentPath string
}

type reviewOptions = reviewdoc.Options

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

type adviseSettings = configdoc.AdviseSettings

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

func backtestResult(args []string, start time.Time) CommandResult {
	options, parseErr := backtestdoc.ParseArgs(args)
	withFormat := func(result CommandResult) CommandResult {
		if options.FormatExplicit && options.Format == "json" {
			result.MachineReadable = true
		}
		return result
	}
	if parseErr != nil {
		return withFormat(errorResult("backtest", "backtest", usageError(parseErr.Message), start))
	}
	wd, err := os.Getwd()
	if err != nil {
		return withFormat(errorResult("backtest", "backtest", internalError("could not inspect working directory", err), start))
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return withFormat(errorResult("backtest", "backtest", configError("could not locate repository root from current directory"), start))
	}
	warnings, commandErr := validateReliaConfig(root)
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}
	config, commandErr := readReliaConfig(root)
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}
	records, sourceArtifacts, sourceDigest, commandErr := loadBacktestExperiences(root)
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}
	windowDays, _ := backtestdoc.ParseWindowDays(options.Window)
	report, commandErr := buildRecurrenceReport(root, config, records, sourceArtifacts, sourceDigest, options, windowDays)
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}
	var baselineSnapshot backtestBaselineSnapshot
	baselineSaved := false
	if options.SaveBaseline && report.Gate.Status != "fail" {
		baselineSnapshot, commandErr = snapshotBacktestBaseline(root, options.BaselinePath)
		if commandErr != nil {
			return withFormat(errorResult("backtest", "backtest", commandErr, start))
		}
		if commandErr := saveBacktestBaseline(root, report, options.BaselinePath); commandErr != nil {
			if rollbackErr := restoreBacktestBaseline(baselineSnapshot); rollbackErr != nil {
				commandErr.Message = commandErr.Message + "; additionally could not roll back ERR baseline: " + rollbackErr.Message
			}
			return withFormat(errorResult("backtest", "backtest", commandErr, start))
		}
		baselineSaved = true
		report.Baseline, commandErr = savedBacktestBaselineComparison(options.BaselinePath, report.HeadlineERR)
		if commandErr != nil {
			return withFormat(errorResult("backtest", "backtest", commandErr, start))
		}
		report.Diagnostics = backtestdoc.BuildReportDiagnostics(report.Summary, report.Baseline, report.SourceArtifacts)
		report.Badge = backtestdoc.BuildReportBadge(report)
	}
	jsonReportPath, htmlReportPath, commandErr := writeBacktestReports(root, report, options.ReportDir)
	if commandErr != nil {
		if baselineSaved {
			if rollbackErr := restoreBacktestBaseline(baselineSnapshot); rollbackErr != nil {
				commandErr.Message = commandErr.Message + "; additionally could not roll back ERR baseline: " + rollbackErr.Message
			}
		}
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}

	result := passResult("backtest", "backtest", "computed conservative recurrence backtest", start, map[string]any{
		"window":                         report.Window,
		"repo_id":                        report.Metadata["repo_id"],
		"experiences_total":              report.Summary.ExperienceCount,
		"experiences_agent_attributed":   report.Metrics.AgentAttributedExperiences,
		"agent_attributed_prs":           report.Metrics.AgentAttributedPRs,
		"agent_failures_by_outcome_kind": report.Metrics.AgentFailuresByOutcomeKind,
		"headline_err":                   report.HeadlineERR,
		"headline_err_percent":           report.Summary.HeadlineERRPercent,
		"error_recurrence_rate":          report.HeadlineERR,
		"recurrences_confirmed":          report.Summary.ConfirmedRecurrenceCount,
		"recurrences_possible":           report.Summary.PossibleRecurrenceCount,
		"confirmed_recurrences":          report.Summary.ConfirmedRecurrenceCount,
		"possible_recurrences":           report.Summary.PossibleRecurrenceCount,
		"flake_discounted_count":         report.Summary.FlakeDiscountedCount,
		"attribution_uncertain_count":    report.Summary.AttributionUncertainCount,
		"top_repeated_mistakes":          report.TopRepeatedMistakes,
		"diagnostics":                    report.Diagnostics,
		"operator_feedback":              report.OperatorFeedback,
		"badge":                          report.Badge,
		"baseline":                       report.Baseline,
		"baseline_ref":                   report.Baseline.Path,
		"gate":                           report.Gate,
		"report":                         report,
		"report_path":                    htmlReportPath,
		"json_report_path":               jsonReportPath,
		"html_report_path":               htmlReportPath,
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs,
		"schemas/recurrence-report.schema.json",
		"docs/product/prd.md#ingest-attribute-backtest-report",
	)
	for _, artifact := range sourceArtifacts {
		result.EvidenceRefs = append(result.EvidenceRefs, artifact)
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "experience_shard", Path: artifact})
	}
	result.Artifacts = append(result.Artifacts,
		ArtifactRef{Kind: "recurrence_report_json", Path: jsonReportPath},
		ArtifactRef{Kind: "recurrence_report_html", Path: htmlReportPath},
	)
	result.RedactionStatus = "applied"
	if report.Gate.Status == "fail" {
		result.Status = "error"
		result.ExitCode = ExitGate
		result.Errors = append(result.Errors, CommandError{
			Type:        "recurrence_gate_failed",
			Message:     fmt.Sprintf("headline ERR %.4f exceeds configured gate %.4f", report.HeadlineERR, backtestdoc.GateThresholdValue(report.Gate)),
			ExitCode:    ExitGate,
			Remediation: "Leave gate.enabled false for advisory-only MVP behavior or raise the explicit threshold after reviewing the recurrence report.",
			Ref:         report.Gate.Ref,
		})
	}
	return withFormat(result)
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
			if commandErr := validateEventMemorySource(event, ref); commandErr != nil {
				return nil, nil, "", commandErr
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
	flakeHeuristics := autoFlakeDiscountedExperiences(windowRecords)
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
			if isFailureOutcome(record.Outcome.Kind) {
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
				Ref:                   sourceLineRef(current),
				Reason:                "Excluded from headline ERR because attribution was ambiguous and the default policy is uncertain: exclude.",
			})
			addBacktestCitation(citationMap, current)
			continue
		}
		if !isFailureOutcome(record.Outcome.Kind) {
			summary.NonFailureOutcomeCount++
			continue
		}
		signatureKeys := recurrenceSignatureKeys(record)
		if record.Attribution.ActorKind != "agent" {
			summary.HumanFailureExcludedCount++
			if record.FlakeDiscount > 0 {
				addBacktestCitation(citationMap, current)
				continue
			}
			appendRecurrencePrior(priorBySignature, signatureKeys, current)
			continue
		}
		summary.AgentFailureDenominator++
		if isBacktestFlakeDiscounted(current, flakeHeuristics) {
			summary.FlakeDiscountedCount++
			flakes = append(flakes, buildBacktestFlakeDiscount(current, windowRecords, flakeHeuristics))
			addBacktestCitation(citationMap, current)
			continue
		}
		if priors := recurrencePriorCandidates(priorBySignature, signatureKeys); len(priors) > 0 {
			prior, confidence, ok := selectRecurrencePrior(priors, current)
			if !ok {
				appendRecurrencePrior(priorBySignature, signatureKeys, current)
				continue
			}
			pair := buildRecurrencePair(prior, current)
			if confidence == "confirmed" {
				pair.Confidence = "confirmed"
				pair.Reason = "Exact reliable signature repeated with overlapping paths."
				confirmed = append(confirmed, pair)
			} else {
				pair.Confidence = "possible"
				pair.Reason = "Exact signature repeated, but extraction confidence or path overlap was insufficient; excluded from the headline ERR."
				possible = append(possible, pair)
			}
			addBacktestCitation(citationMap, prior)
			addBacktestCitation(citationMap, current)
		}
		appendRecurrencePrior(priorBySignature, signatureKeys, current)
	}
	sortRecurrencePairs(confirmed)
	sortRecurrencePairs(possible)
	sortBacktestFlakes(flakes)
	sortBacktestUncertain(uncertain)
	citations := backtestCitations(citationMap)
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

func isFailureOutcome(kind string) bool {
	switch kind {
	case "ci_failure", "revert", "review_correction":
		return true
	default:
		return false
	}
}

func reliableSignatureExtraction(value string) bool {
	return value == "structured" || value == "log_parsed_high"
}

func recurrenceSignatureKeys(record experienceRecord) []string {
	signatureMetadata, _ := record.Metadata["signature"].(map[string]any)
	signatureClass := strings.TrimSpace(stringFromAny(signatureMetadata["class"]))
	signatureKey := strings.TrimSpace(stringFromAny(signatureMetadata["key"]))
	messageFingerprint := strings.TrimSpace(stringFromAny(signatureMetadata["message_fingerprint"]))
	keys := []string{}
	if signatureClass != "" && signatureKey != "" {
		keys = append(keys, strings.Join([]string{"class_key", signatureClass, signatureKey}, "\x00"))
	}
	if messageFingerprint != "" {
		keys = append(keys, strings.Join([]string{"message", messageFingerprint}, "\x00"))
	}
	if len(keys) == 0 {
		keys = append(keys, strings.Join([]string{"id", record.Outcome.Signature.SignatureID}, "\x00"))
	}
	return keys
}

func matchedRecurrenceSignatureID(left experienceRecord, right experienceRecord) string {
	leftSignatureID := strings.TrimSpace(left.Outcome.Signature.SignatureID)
	rightSignatureID := strings.TrimSpace(right.Outcome.Signature.SignatureID)
	rightKeys := map[string]bool{}
	for _, key := range recurrenceSignatureKeys(right) {
		rightKeys[key] = true
	}
	for _, key := range recurrenceSignatureKeys(left) {
		if rightKeys[key] {
			return displayRecurrenceSignatureKey(key, rightSignatureID)
		}
	}
	if leftSignatureID != "" && leftSignatureID == rightSignatureID {
		return rightSignatureID
	}
	if rightSignatureID != "" {
		return rightSignatureID
	}
	return leftSignatureID
}

func displayRecurrenceSignatureKey(key string, fallback string) string {
	parts := strings.Split(key, "\x00")
	switch {
	case len(parts) == 3 && parts[0] == "class_key":
		return strings.Join([]string{"class_key", parts[1], parts[2]}, ":")
	case len(parts) == 2 && parts[0] == "message":
		return "message:" + parts[1]
	case len(parts) == 2 && parts[0] == "id":
		return parts[1]
	case fallback != "":
		return fallback
	default:
		return strings.ReplaceAll(key, "\x00", ":")
	}
}

func appendRecurrencePrior(priorBySignature map[string][]backtestExperience, keys []string, current backtestExperience) {
	for _, key := range keys {
		priorBySignature[key] = append(priorBySignature[key], current)
	}
}

func recurrencePriorCandidates(priorBySignature map[string][]backtestExperience, keys []string) []backtestExperience {
	seen := map[string]bool{}
	priors := []backtestExperience{}
	for _, key := range keys {
		for _, prior := range priorBySignature[key] {
			experienceID := prior.Record.ExperienceID
			if experienceID == "" {
				experienceID = sourceLineRef(prior)
			}
			if seen[experienceID] {
				continue
			}
			seen[experienceID] = true
			priors = append(priors, prior)
		}
	}
	sort.Slice(priors, func(i, j int) bool {
		if priors[i].RecordedAt.Equal(priors[j].RecordedAt) {
			return priors[i].Record.ExperienceID < priors[j].Record.ExperienceID
		}
		return priors[i].RecordedAt.Before(priors[j].RecordedAt)
	})
	return priors
}

func recordsShareRecurrenceSignature(left experienceRecord, right experienceRecord) bool {
	rightKeys := map[string]bool{}
	for _, key := range recurrenceSignatureKeys(right) {
		rightKeys[key] = true
	}
	for _, key := range recurrenceSignatureKeys(left) {
		if rightKeys[key] {
			return true
		}
	}
	return false
}

func confirmedRecurrence(prior backtestExperience, current backtestExperience) bool {
	if prior.Record.Action.PR == current.Record.Action.PR {
		return false
	}
	if prior.Record.Outcome.Signature.SignatureID == "" ||
		!recordsShareRecurrenceSignature(prior.Record, current.Record) {
		return false
	}
	if !reliableSignatureExtraction(prior.Record.Outcome.Signature.ExtractionConfidence) ||
		!reliableSignatureExtraction(current.Record.Outcome.Signature.ExtractionConfidence) {
		return false
	}
	return pathSetsOverlap(prior.Record.Context.Paths, current.Record.Context.Paths)
}

func selectRecurrencePrior(priors []backtestExperience, current backtestExperience) (backtestExperience, string, bool) {
	for index := len(priors) - 1; index >= 0; index-- {
		if priors[index].Record.Action.PR == current.Record.Action.PR {
			continue
		}
		if confirmedRecurrence(priors[index], current) {
			return priors[index], "confirmed", true
		}
	}
	for index := len(priors) - 1; index >= 0; index-- {
		if priors[index].Record.Action.PR != current.Record.Action.PR {
			return priors[index], "possible", true
		}
	}
	return backtestExperience{}, "", false
}

func pathSetsOverlap(left []string, right []string) bool {
	leftSet := map[string]bool{}
	for _, value := range left {
		if clean, ok := configdoc.CleanRepoPath(value); ok {
			leftSet[filepath.ToSlash(clean)] = true
		}
	}
	for _, value := range right {
		if clean, ok := configdoc.CleanRepoPath(value); ok && leftSet[filepath.ToSlash(clean)] {
			return true
		}
	}
	return false
}

func buildRecurrencePair(prior backtestExperience, current backtestExperience) recurrencePair {
	return recurrencePair{
		CurrentExperienceID: current.Record.ExperienceID,
		PriorExperienceID:   prior.Record.ExperienceID,
		CurrentPR:           current.Record.Action.PR,
		PriorPR:             prior.Record.Action.PR,
		CurrentURL:          ingestdoc.PrimaryProvenanceURL(current.Record),
		PriorURL:            ingestdoc.PrimaryProvenanceURL(prior.Record),
		SignatureID:         current.Record.Outcome.Signature.SignatureID,
		MatchedSignatureID:  matchedRecurrenceSignatureID(prior.Record, current.Record),
		Refs:                []string{sourceLineRef(prior), sourceLineRef(current)},
	}
}

func autoFlakeDiscountedExperiences(records []backtestExperience) map[string]string {
	bySignature := map[string][]backtestExperience{}
	for _, record := range records {
		if record.Record.Attribution.ActorKind != "agent" || !isFailureOutcome(record.Record.Outcome.Kind) {
			continue
		}
		for _, key := range recurrenceSignatureKeys(record.Record) {
			bySignature[key] = append(bySignature[key], record)
		}
	}
	discounted := map[string]string{}
	for _, group := range bySignature {
		if len(group) < 3 || !groupHasUnrelatedNonTestDiffs(group) {
			continue
		}
		reason := "Discounted as flaky because the same failure signature appears at least three times across unrelated non-test diff paths."
		for _, record := range group {
			if record.Record.FlakeDiscount == 0 && discounted[record.Record.ExperienceID] == "" {
				discounted[record.Record.ExperienceID] = reason
			}
		}
	}
	return discounted
}

func groupHasUnrelatedNonTestDiffs(group []backtestExperience) bool {
	seen := map[string]bool{}
	for _, record := range group {
		paths := distilldoc.NonTestPaths(record.Record.Context.Paths)
		if len(paths) == 0 {
			paths = distilldoc.NormalizedRepoPaths(record.Record.Context.Paths)
		}
		for _, path := range paths {
			if seen[path] {
				return false
			}
			seen[path] = true
		}
	}
	return len(seen) >= len(group)
}

func isBacktestFlakeDiscounted(record backtestExperience, heuristics map[string]string) bool {
	return record.Record.FlakeDiscount > 0 || heuristics[record.Record.ExperienceID] != ""
}

func buildBacktestFlakeDiscount(record backtestExperience, records []backtestExperience, heuristics map[string]string) backtestFlakeDiscount {
	supportingPRs := []int{}
	supportingRefs := []string{}
	for _, candidate := range records {
		if candidate.Record.ExperienceID == record.Record.ExperienceID {
			continue
		}
		if !recordsShareRecurrenceSignature(candidate.Record, record.Record) {
			continue
		}
		if candidate.Record.Attribution.ActorKind != "agent" || !isFailureOutcome(candidate.Record.Outcome.Kind) {
			continue
		}
		supportingPRs = append(supportingPRs, candidate.Record.Action.PR)
		supportingRefs = append(supportingRefs, sourceLineRef(candidate))
	}
	sort.Ints(supportingPRs)
	supportingRefs = uniqueStrings(supportingRefs)
	reason := heuristics[record.Record.ExperienceID]
	flakeDiscount := record.Record.FlakeDiscount
	if reason == "" {
		reason = "Discounted as flaky because the experience record carries an explicit flake_discount."
	}
	if flakeDiscount == 0 {
		flakeDiscount = 1
	}
	return backtestFlakeDiscount{
		ExperienceID:    record.Record.ExperienceID,
		PR:              record.Record.Action.PR,
		SignatureID:     record.Record.Outcome.Signature.SignatureID,
		FlakeDiscount:   roundFloat(flakeDiscount, 4),
		SupportingPRs:   supportingPRs,
		SupportingRefs:  supportingRefs,
		Reason:          reason,
		ExcludedFromERR: true,
	}
}

func addBacktestCitation(citations map[int]backtestCitation, record backtestExperience) {
	url := ingestdoc.PrimaryProvenanceURL(record.Record)
	if url == "" {
		return
	}
	citations[record.Record.Action.PR] = backtestCitation{
		PR:           record.Record.Action.PR,
		URL:          url,
		ExperienceID: record.Record.ExperienceID,
	}
}

func sourceLineRef(record backtestExperience) string {
	return fmt.Sprintf("%s:%d", record.SourcePath, record.SourceLine)
}

func sortRecurrencePairs(pairs []recurrencePair) {
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].CurrentPR == pairs[j].CurrentPR {
			return pairs[i].CurrentExperienceID < pairs[j].CurrentExperienceID
		}
		return pairs[i].CurrentPR < pairs[j].CurrentPR
	})
}

func sortBacktestFlakes(flakes []backtestFlakeDiscount) {
	sort.Slice(flakes, func(i, j int) bool {
		if flakes[i].PR == flakes[j].PR {
			return flakes[i].ExperienceID < flakes[j].ExperienceID
		}
		return flakes[i].PR < flakes[j].PR
	})
}

func sortBacktestUncertain(uncertain []backtestUncertain) {
	sort.Slice(uncertain, func(i, j int) bool {
		if uncertain[i].PR == uncertain[j].PR {
			return uncertain[i].ExperienceID < uncertain[j].ExperienceID
		}
		return uncertain[i].PR < uncertain[j].PR
	})
}

func backtestCitations(citationMap map[int]backtestCitation) []backtestCitation {
	citations := make([]backtestCitation, 0, len(citationMap))
	for _, citation := range citationMap {
		citations = append(citations, citation)
	}
	sort.Slice(citations, func(i, j int) bool {
		return citations[i].PR < citations[j].PR
	})
	return citations
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

func distillInputPathMetadata(options distillOptions, sourceArtifacts []string) string {
	if strings.TrimSpace(options.InputPath) == "" {
		return ""
	}
	if len(sourceArtifacts) == 0 {
		return ""
	}
	return sourceArtifacts[0]
}

func reviewResult(args []string, start time.Time) CommandResult {
	options, parseErr := reviewdoc.ParseArgs(args)
	if parseErr != nil {
		return errorResult("review", "review", usageError(parseErr.Message), start)
	}
	wd, err := os.Getwd()
	if err != nil {
		return errorResult("review", "review", internalError("could not inspect working directory", err), start)
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return errorResult("review", "review", configError("could not locate repository root from current directory"), start)
	}
	warnings, commandErr := validateReliaConfig(root)
	if commandErr != nil {
		return errorResult("review", "review", commandErr, start)
	}
	updateOptions := reviewUpdateOptions()
	rulePath, commandErr := reviewdoc.FindRulePath(root, "memory/rules", options.Rule, updateOptions)
	if commandErr != nil {
		return errorResult("review", "review", commandErr, start)
	}
	status, commandErr := reviewdoc.UpdateRuleReview(root, rulePath, options, updateOptions)
	if commandErr != nil {
		return errorResult("review", "review", commandErr, start)
	}
	rel := displayPath(root, rulePath)
	result := passResult("review", "review", "updated memory rule review label", start, map[string]any{
		"rule":         options.Rule,
		"rule_path":    rel,
		"action":       options.Action,
		"review_label": options.Label,
		"status":       status,
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs, "schemas/memory-rule.schema.json", rel)
	result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "memory_rule", Path: rel})
	return result
}

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

func adviseResult(args []string, start time.Time) CommandResult {
	options, commandErr := parseAdviseArgs(args)
	withFormat := func(result CommandResult) CommandResult {
		if options.Format == "json" {
			result.MachineReadable = true
		}
		return result
	}
	if commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
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
	touchedPaths, commandErr := parseUnifiedDiffTouchedPaths(inputContent, displayPath(root, inputPath))
	if commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	rules, commandErr := assessdoc.LoadRules(root, assessmentBuildOptions())
	if commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	assessment, commandErr := assessdoc.BuildRiskAssessment(root, displayPath(root, inputPath), inputContent, touchedPaths, rules, assessmentBuildOptions())
	if commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	diffFingerprint := sha256String(string(inputContent))
	previousState, commandErr := advisoryPreviousState(root, options.StatePath)
	if commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	shouldComment, skipReason := advisedoc.CommentDecision(settings, assessment, diffFingerprint, previousState, start)
	body := ""
	if shouldComment {
		body = advisedoc.RenderComment(assessment, touchedPaths, diffFingerprint, start, skipReason)
		if commandErr := writeRepoRelativeFile(root, options.CommentPath, []byte(body), "advisory comment"); commandErr != nil {
			return withFormat(errorResult("advise", "advise", commandErr, start))
		}
	}
	generatedAt := start.UTC().Format(time.RFC3339)
	publishedRiskLevel := advisedoc.PublishedRiskLevel(assessment, skipReason)
	stateDiffFingerprint := diffFingerprint
	stateGeneratedAt := generatedAt
	stateMetadata := map[string]any{
		"generated_by":              "relia advise",
		"generated_at":              stateGeneratedAt,
		"hosted_service_required":   false,
		"github_api_required_later": shouldComment,
		"risk_level":                publishedRiskLevel,
	}
	if skipReason == "reassess_debounce_window" && previousState.DiffFingerprint != "" {
		stateDiffFingerprint = previousState.DiffFingerprint
		if !previousState.GeneratedAt.IsZero() {
			stateGeneratedAt = previousState.GeneratedAt.UTC().Format(time.RFC3339)
			stateMetadata["generated_at"] = stateGeneratedAt
		}
		stateMetadata["debounced_diff_fingerprint"] = diffFingerprint
		stateMetadata["debounced_at"] = generatedAt
	}
	state := map[string]any{
		"object_type":               "relia.advisory_state",
		"schema_version":            commandSchemaVersion,
		"input_path":                displayPath(root, inputPath),
		"diff_fingerprint":          stateDiffFingerprint,
		"previous_diff_fingerprint": previousState.DiffFingerprint,
		"should_comment":            shouldComment,
		"skip_reason":               skipReason,
		"assessment":                assessment,
		"comment_strategy":          advisedoc.CommentStrategy(settings),
		"comment_marker":            "relia-advisory:v1",
		"metadata":                  stateMetadata,
	}
	encodedState, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return withFormat(errorResult("advise", "advise", internalError("could not encode advisory state", err), start))
	}
	if commandErr := writeRepoRelativeFile(root, options.StatePath, append(encodedState, '\n'), "advisory state"); commandErr != nil {
		return withFormat(errorResult("advise", "advise", commandErr, start))
	}
	result := passResult("advise", "advise", "planned advisory PR comment from local assessment", start, map[string]any{
		"input_path":         displayPath(root, inputPath),
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
		"comment_strategy":   advisedoc.CommentStrategy(settings),
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs,
		"schemas/risk-assessment.schema.json",
		displayPath(root, inputPath),
		options.StatePath,
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

func parseAdviseArgs(args []string) (adviseOptions, *CommandError) {
	options := adviseOptions{
		Format:      "json",
		StatePath:   ".relia/reports/advisory-state.json",
		CommentPath: ".relia/reports/advisory-comment.md",
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--input", "--diff", "-i":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("advise requires a path after " + arg)
			}
			options.InputPath = args[index+1]
			index++
		case "--format":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("advise requires a value after --format")
			}
			options.Format = args[index+1]
			index++
		case "--state":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("advise requires a repo-relative path after --state")
			}
			options.StatePath = args[index+1]
			index++
		case "--comment":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("advise requires a repo-relative path after --comment")
			}
			options.CommentPath = args[index+1]
			index++
		default:
			return options, usageError(fmt.Sprintf("unknown advise argument %q", arg))
		}
	}
	if strings.TrimSpace(options.InputPath) == "" {
		return options, usageError("advise requires --input <diff> in offline mode")
	}
	if options.Format != "json" {
		return options, usageError("advise only supports --format json in this task slice")
	}
	for _, item := range []struct {
		label string
		path  string
	}{
		{"advise --state", options.StatePath},
		{"advise --comment", options.CommentPath},
	} {
		if _, ok := configdoc.CleanRepoPath(item.path); !ok {
			return options, usageError(item.label + " must be repo-relative")
		}
	}
	return options, nil
}

type advisoryPriorState = advisedoc.PriorState

func advisoryPreviousState(root string, statePath string) (advisoryPriorState, *CommandError) {
	clean, ok := configdoc.CleanRepoPath(statePath)
	if !ok {
		return advisoryPriorState{}, usageError("advise --state must be repo-relative")
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(filepath.ToSlash(clean))))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return advisoryPriorState{}, nil
		}
		return advisoryPriorState{}, internalError("could not read prior advisory state", err)
	}
	var state map[string]any
	if err := json.Unmarshal(content, &state); err != nil {
		return advisoryPriorState{}, artifactContractError("prior advisory state is not valid JSON", filepath.ToSlash(clean))
	}
	prior := advisoryPriorState{}
	fingerprint, _ := state["diff_fingerprint"].(string)
	prior.DiffFingerprint = fingerprint
	prior.SkipReason, _ = state["skip_reason"].(string)
	if metadata, ok := state["metadata"].(map[string]any); ok {
		if generatedAt, _ := metadata["generated_at"].(string); generatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, generatedAt); err == nil {
				prior.GeneratedAt = parsed
			}
		}
		prior.RiskLevel, _ = metadata["risk_level"].(string)
	}
	if assessment, ok := state["assessment"].(map[string]any); ok {
		if riskLevel, _ := assessment["risk_level"].(string); riskLevel != "" && prior.RiskLevel == "" {
			prior.RiskLevel = riskLevel
		}
	}
	return prior, nil
}

func writeRepoRelativeFile(root string, rel string, content []byte, label string) *CommandError {
	clean, ok := configdoc.CleanRepoPath(rel)
	if !ok {
		return usageError(label + " path must be repo-relative")
	}
	return writeAtomicRepoFile(filepath.Join(root, filepath.FromSlash(filepath.ToSlash(clean))), content, label)
}

func modelsResult(args []string, start time.Time) CommandResult {
	if len(args) == 0 || args[0] != "pull" {
		return errorResult("models", "models", usageError("expected subcommand: pull"), start)
	}
	options, parseErr := modelpulldoc.ParseArgs(args[1:])
	if parseErr != nil {
		return errorResult("models pull", "models", usageError(parseErr.Message), start)
	}
	wd, err := os.Getwd()
	if err != nil {
		return errorResult("models pull", "models", internalError("could not inspect working directory", err), start)
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return errorResult("models pull", "models", configError("could not locate repository root from current directory"), start)
	}
	config, commandErr := readReliaConfig(root)
	if commandErr != nil {
		return errorResult("models pull", "models", commandErr, start)
	}
	manifestRel := ".relia/models/manifest.json"
	if scalar, ok := config.Scalars["models.local_manifest"]; ok {
		manifestRel = scalar.Value
	}
	cleanManifestRel, ok := configdoc.CleanRepoPath(manifestRel)
	if !ok {
		return errorResult("models pull", "models", dependencyError("local model manifest path must be repo-relative", defaultConfigFile), start)
	}
	manifestDisplayPath := filepath.ToSlash(filepath.Clean(cleanManifestRel))
	cleanCachePath, ok := configdoc.CleanRepoPath(options.CachePath)
	if !ok {
		return errorResult("models pull", "models", usageError("models pull --cache-path must be repo-relative"), start)
	}
	cachePath := filepath.ToSlash(filepath.Clean(cleanCachePath))
	if cachePath == manifestDisplayPath {
		return errorResult("models pull", "models", usageError("models pull --cache-path must not equal the local model manifest path"), start)
	}
	manifest := modelpulldoc.Manifest(options, cachePath)
	if commandErr := validateLocalModelManifestPayload(root, manifest, manifestRel); commandErr != nil {
		return errorResult("models pull", "models", commandErr, start)
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return errorResult("models pull", "models", internalError("could not encode local model manifest", err), start)
	}
	manifestPath := filepath.Join(root, filepath.FromSlash(manifestDisplayPath))
	if commandErr := writeAtomicRepoFile(manifestPath, append(encoded, '\n'), "local model manifest"); commandErr != nil {
		return errorResult("models pull", "models", commandErr, start)
	}
	result := passResult("models pull", "models", "recorded local embedding model artifact manifest", start, map[string]any{
		"model_id":        manifest.ModelID,
		"version":         manifest.Version,
		"source_url":      manifest.SourceURL,
		"license":         manifest.License,
		"digest":          manifest.Digest,
		"cache_path":      manifest.CachePath,
		"update_policy":   manifest.UpdatePolicy,
		"rollback_policy": manifest.RollbackPolicy,
		"manifest_path":   manifestDisplayPath,
		"network_used":    false,
		"status":          manifest.Status,
	})
	result.EvidenceRefs = append(result.EvidenceRefs,
		"docs/dev/dev_guides.md#model-provider-and-artifact-policy",
		manifestDisplayPath,
	)
	result.Artifacts = append(result.Artifacts,
		ArtifactRef{Kind: "local_model_manifest", Path: manifestDisplayPath},
		ArtifactRef{Kind: "local_model_artifact", Path: manifest.CachePath},
	)
	return result
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
	flakeHeuristics := autoFlakeDiscountedExperiences(records)
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

func writeDistilledRules(root string, ruleDir string, rules []distilledRule) ([]ArtifactRef, *CommandError) {
	cleanRuleDir, ok := configdoc.CleanRepoPath(ruleDir)
	if !ok {
		return nil, usageError("distill rule directory must be repo-relative")
	}
	ruleDirRel := filepath.ToSlash(cleanRuleDir)
	ruleDirPath := filepath.Join(root, filepath.FromSlash(ruleDirRel))
	if err := os.MkdirAll(ruleDirPath, 0o755); err != nil {
		return nil, internalError("could not create memory rule directory", err)
	}
	artifacts := make([]ArtifactRef, 0, len(rules))
	for _, rule := range rules {
		rule = mergeExistingRuleLifecycle(root, ruleDirRel, rule)
		rel := filepath.ToSlash(filepath.Join(ruleDirRel, rule.ID+".yaml"))
		path := filepath.Join(root, filepath.FromSlash(rel))
		content := []byte(distilldoc.RenderRuleYAML(rule))
		if commandErr := writeAtomicRepoFile(path, content, "memory rule"); commandErr != nil {
			return nil, commandErr
		}
		artifacts = append(artifacts, ArtifactRef{Kind: "memory_rule", Path: rel})
	}
	return artifacts, nil
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
	if document.Scalars["status"].Value == "active" &&
		document.Scalars["review.label"].Value == "accepted" &&
		rule.Status == "candidate" {
		rule.Status = "active"
		rule.ReviewLabel = "accepted"
		rule.Metadata.LifecycleReason = "previous accepted review preserved"
	}
	return rule
}

func yamlFloat(value float64) string {
	return distilldoc.YAMLFloat(value)
}

func writeAtomicRepoFile(path string, content []byte, label string) *CommandError {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return internalError("could not create "+label+" directory", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return internalError("could not create temporary "+label, err)
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
		return internalError("could not write temporary "+label, err)
	}
	if err := tempFile.Close(); err != nil {
		return internalError("could not close temporary "+label, err)
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return internalError("could not set temporary "+label+" permissions", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return internalError("could not write "+label, err)
	}
	cleanup = false
	return nil
}

func writeMemoryPage(root string, outputPath string, rules []memorydoc.RuleSummary) (string, *CommandError) {
	clean, ok := configdoc.CleanRepoPath(outputPath)
	if !ok {
		return "", usageError("memory output path must be repo-relative")
	}
	rel := filepath.ToSlash(clean)
	path := filepath.Join(root, filepath.FromSlash(rel))
	if commandErr := writeAtomicRepoFile(path, []byte(memorydoc.RenderMarkdown(rules)), "memory page"); commandErr != nil {
		return "", commandErr
	}
	return rel, nil
}

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

func readReliaConfig(root string) (yamlDocument, *CommandError) {
	document, configErr := configdoc.Read(root)
	return document, commandErrorFromConfig(configErr)
}

func resolveInputPath(root string, input string) string {
	if filepath.IsAbs(input) {
		return filepath.Clean(input)
	}
	return filepath.Clean(filepath.Join(root, input))
}

func parseIngestEvents(content []byte, ref string) ([]map[string]any, *CommandError) {
	events, ingestErr := ingestdoc.ParseEvents(content, ref)
	return events, commandErrorFromIngest(ingestErr)
}

func decodeJSONUseNumber(input string, target any) error {
	return ingestdoc.DecodeJSONUseNumber(input, target)
}

func normalizeExperienceRecord(config yamlDocument, event map[string]any, index int, ref string) (experienceRecord, bool, *CommandError) {
	if commandErr := validateEventMemorySource(event, ref); commandErr != nil {
		return experienceRecord{}, false, commandErr
	}
	repo, commandErr := normalizeExperienceRepo(event, ref)
	if commandErr != nil {
		return experienceRecord{}, false, commandErr
	}
	recordedAt := stringField(event, "recorded_at")
	if recordedAt == "" {
		return experienceRecord{}, false, artifactContractError("experience record missing recorded_at", ref)
	}
	parsedRecordedAt, err := time.Parse(time.RFC3339, recordedAt)
	if err != nil {
		return experienceRecord{}, false, artifactContractError("experience record recorded_at must be RFC3339", ref)
	}
	action, commandErr := normalizeExperienceAction(event, ref)
	if commandErr != nil {
		return experienceRecord{}, false, commandErr
	}
	attribution, skipped, commandErr := normalizeExperienceAttribution(config, event, ref)
	if commandErr != nil || skipped {
		return experienceRecord{}, skipped, commandErr
	}
	paths := stringListField(event, "paths", "context.paths")
	if len(paths) == 0 {
		return experienceRecord{}, false, artifactContractError("experience record must include at least one context path", ref)
	}
	context := experienceContext{
		Paths:           paths,
		DiffFingerprint: stringField(event, "diff_fingerprint", "context.diff_fingerprint"),
	}
	if context.DiffFingerprint == "" {
		context.DiffFingerprint = sha256String(fmt.Sprintf("%d|%s|%s", action.PR, action.Commit, strings.Join(paths, "|")))
	}
	outcome, signatureMetadata, commandErr := normalizeExperienceOutcome(event, action, paths, ref)
	if commandErr != nil {
		return experienceRecord{}, false, commandErr
	}
	provenance, commandErr := normalizeExperienceProvenance(event, ref)
	if commandErr != nil {
		return experienceRecord{}, false, commandErr
	}
	flakeDiscount := 0.0
	if parsedFlakeDiscount, exists, commandErr := optionalFloatField(event, ref, "flake_discount", "flake_discount"); commandErr != nil {
		return experienceRecord{}, false, commandErr
	} else if exists {
		flakeDiscount = parsedFlakeDiscount
	}
	if flakeDiscount < 0 || flakeDiscount > 1 {
		return experienceRecord{}, false, artifactContractError("flake_discount must be between 0 and 1", ref)
	}
	metadata := metadataField(event)
	metadata["source_input_index"] = index
	metadata["source_kind"] = "local_input"
	metadata["memory_source"] = "verified_outcome_event"
	metadata["signature"] = signatureMetadata
	experienceID := stringField(event, "experience_id")
	if experienceID == "" {
		experienceID = generatedExperienceID(action, parsedRecordedAt, outcome, signatureMetadata, provenance)
	}
	return experienceRecord{
		ObjectType:      "relia.experience_record",
		SchemaVersion:   commandSchemaVersion,
		ExperienceID:    experienceID,
		Repo:            repo,
		RecordedAt:      parsedRecordedAt.UTC().Format(time.RFC3339),
		Attribution:     attribution,
		Context:         context,
		Action:          action,
		Outcome:         outcome,
		Provenance:      provenance,
		FlakeDiscount:   flakeDiscount,
		OrgEligible:     false,
		ShareScope:      "private",
		RedactionStatus: "applied",
		Metadata:        metadata,
	}, false, nil
}

func validateEventMemorySource(event map[string]any, ref string) *CommandError {
	return commandErrorFromIngest(ingestdoc.ValidateEventMemorySource(event, ref))
}

func generatedExperienceID(action experienceAction, recordedAt time.Time, outcome experienceOutcome, signatureMetadata map[string]any, provenance experienceProvenance) string {
	provenanceURLs := append([]string(nil), provenance.URLs...)
	sort.Strings(provenanceURLs)
	identityParts := []string{
		strconv.Itoa(action.PR),
		action.Commit,
		recordedAt.UTC().Format(time.RFC3339),
		outcome.Kind,
		outcome.TerminalState,
		outcome.Signature.SignatureID,
		outcome.Signature.ExtractionConfidence,
		stringFromAny(signatureMetadata["class"]),
		stringFromAny(signatureMetadata["check_name"]),
		stringFromAny(signatureMetadata["key"]),
		stringFromAny(signatureMetadata["message_fingerprint"]),
	}
	identityParts = append(identityParts, provenanceURLs...)
	return fmt.Sprintf("exp_%04d_%s", action.PR, shortHash(strings.Join(identityParts, "\x00")))
}

func normalizeExperienceRepo(event map[string]any, ref string) (experienceRepo, *CommandError) {
	repo := experienceRepo{Provider: "github"}
	if value, ok := nestedField(event, "repo"); ok {
		switch typed := value.(type) {
		case map[string]any:
			if provider := stringFromAny(typed["provider"]); provider != "" {
				repo.Provider = provider
			}
			repo.Owner = stringFromAny(typed["owner"])
			repo.Name = stringFromAny(typed["name"])
		case string:
			owner, name, ok := strings.Cut(typed, "/")
			if ok {
				repo.Owner = strings.TrimSpace(owner)
				repo.Name = strings.TrimSpace(name)
			}
		}
	}
	if repo.Owner == "" {
		repo.Owner = stringField(event, "repo_owner")
	}
	if repo.Name == "" {
		repo.Name = stringField(event, "repo_name")
	}
	if repo.Provider != "github" {
		return repo, artifactContractError("experience repo.provider must be github", ref)
	}
	if repo.Owner == "" || repo.Name == "" {
		return repo, artifactContractError("experience repo must include owner and name", ref)
	}
	return repo, nil
}

func normalizeExperienceAction(event map[string]any, ref string) (experienceAction, *CommandError) {
	pr, commandErr := requiredPositiveIntField(event, ref, "experience record PR number", "pr", "action.pr")
	if commandErr != nil {
		return experienceAction{}, commandErr
	}
	action := experienceAction{
		PR:     pr,
		Commit: stringField(event, "commit", "action.commit"),
	}
	if action.Commit == "" {
		commits := stringListField(event, "commits", "action.commits")
		if len(commits) > 0 {
			action.Commit = commits[0]
		}
	}
	if action.Commit == "" {
		return action, artifactContractError("experience record must include commit", ref)
	}
	return action, nil
}

func normalizeExperienceAttribution(config yamlDocument, event map[string]any, ref string) (experienceAttribution, bool, *CommandError) {
	confidence := -1.0
	if parsedConfidence, exists, commandErr := optionalFloatField(event, ref, "attribution confidence", "attribution_confidence", "attribution.confidence"); commandErr != nil {
		return experienceAttribution{}, false, commandErr
	} else if exists {
		confidence = parsedConfidence
	}
	attribution := experienceAttribution{
		ActorKind:  stringField(event, "actor_kind", "attribution.actor_kind"),
		Method:     stringField(event, "attribution_method", "attribution.method"),
		Confidence: confidence,
	}
	if attribution.ActorKind == "" {
		switch {
		case overlaps(stringListField(event, "labels", "pr_labels"), yamlListValues(config, "attribution.pr_labels")):
			attribution.ActorKind = "agent"
			attribution.Method = "pr_label"
		case overlaps(stringListField(event, "coauthors", "coauthor_trailers"), yamlListValues(config, "attribution.coauthor_trailers")):
			attribution.ActorKind = "agent"
			attribution.Method = "coauthor_trailer"
		case containsStringValue(yamlListValuesWithMapFields(config, "attribution.agent_authors", "login"), attributionActorLogin(event)):
			attribution.ActorKind = "agent"
			attribution.Method = "bot_login"
		default:
			attribution.ActorKind = "uncertain"
			attribution.Method = "uncertain"
		}
	}
	switch attribution.ActorKind {
	case "agent", "human", "uncertain":
	default:
		return attribution, false, artifactContractError("attribution actor_kind must be agent, human, or uncertain", ref)
	}
	if attribution.ActorKind == "uncertain" && attributionUncertainPolicy(config) == "exclude" {
		return attribution, true, nil
	}
	if attribution.Method == "" {
		if attribution.ActorKind == "human" {
			attribution.Method = "manual"
		} else {
			attribution.Method = "uncertain"
		}
	}
	switch attribution.Method {
	case "bot_login", "coauthor_trailer", "pr_label", "manual", "uncertain":
	default:
		return attribution, false, artifactContractError("attribution method is invalid", ref)
	}
	if attribution.Confidence < 0 {
		attribution.Confidence = defaultAttributionConfidence(attribution.Method)
	}
	if attribution.Confidence < 0 || attribution.Confidence > 1 {
		return attribution, false, artifactContractError("attribution confidence must be between 0 and 1", ref)
	}
	return attribution, false, nil
}

func attributionActorLogin(event map[string]any) string {
	return stringField(event, "actor.login", "author.login", "actor", "author")
}

func attributionUncertainPolicy(document yamlDocument) string {
	if scalar, ok := document.Scalars["attribution.uncertain"]; ok {
		switch scalar.Value {
		case "include_flagged":
			return "include_flagged"
		case "exclude":
			return "exclude"
		}
	}
	return "exclude"
}

func normalizeExperienceOutcome(event map[string]any, action experienceAction, paths []string, ref string) (experienceOutcome, map[string]any, *CommandError) {
	kind := stringField(event, "outcome_kind", "outcome.kind")
	if !validOutcomeKind(kind) {
		return experienceOutcome{}, nil, artifactContractError("outcome kind is invalid", ref)
	}
	terminalState := stringField(event, "terminal_state", "terminal", "outcome.terminal_state", "outcome.terminal")
	if terminalState == "" {
		terminalState = terminalStateForOutcome(kind)
	}
	if !validTerminalState(terminalState) {
		return experienceOutcome{}, nil, artifactContractError("outcome terminal_state is invalid", ref)
	}
	signatureClass := stringField(event, "signature_class", "outcome.signature.class")
	if signatureClass == "" {
		signatureClass = signatureClassForOutcome(kind)
	}
	checkName := stringField(event, "check_name", "outcome.signature.check", "outcome.signature.check_name")
	if checkName == "" {
		checkName = kind
	}
	signatureKey := stringField(event, "signature_key", "outcome.signature.key")
	if signatureKey == "" && len(paths) > 0 {
		signatureKey = paths[0]
	}
	if signatureKey == "" {
		signatureKey = action.Commit
	}
	extractionConfidence := stringField(event, "extraction_confidence", "outcome.signature.extraction_confidence")
	if extractionConfidence == "" {
		extractionConfidence = "structured"
	}
	if !validExtractionConfidence(extractionConfidence) {
		return experienceOutcome{}, nil, artifactContractError("signature extraction_confidence is invalid", ref)
	}
	messageFingerprint := stringField(event, "message_fingerprint", "outcome.signature.message_fingerprint")
	if messageFingerprint == "" {
		message := stringField(event, "message", "log", "outcome.message")
		if message != "" {
			messageFingerprint = sha256String(strings.TrimSpace(message))
		}
	}
	signatureID := stringField(event, "signature_id", "outcome.signature.signature_id")
	if signatureID == "" {
		signatureID = "sig_" + shortHash(signatureClass+"|"+checkName+"|"+signatureKey)
	}
	metadata := map[string]any{
		"class":             signatureClass,
		"check_name":        checkName,
		"key":               signatureKey,
		"extraction_method": extractionMethodForConfidence(extractionConfidence),
	}
	if messageFingerprint != "" {
		metadata["message_fingerprint"] = messageFingerprint
	}
	return experienceOutcome{
		Kind:          kind,
		TerminalState: terminalState,
		Signature: experienceSignature{
			SignatureID:          signatureID,
			ExtractionConfidence: extractionConfidence,
		},
	}, metadata, nil
}

func normalizeExperienceProvenance(event map[string]any, ref string) (experienceProvenance, *CommandError) {
	urls := stringListField(event, "provenance_urls", "provenance.urls")
	for _, key := range []string{"pr_url", "check_run_url", "revert_url", "review_url"} {
		if value := stringField(event, key, "provenance."+key); value != "" {
			urls = append(urls, value)
		}
	}
	urls = uniqueStrings(urls)
	if len(urls) == 0 {
		return experienceProvenance{}, provenanceIntegrityError("experience record must include at least one provenance URL", ref)
	}
	for _, value := range urls {
		if !validGitHubProvenanceURLShape(value) {
			return experienceProvenance{}, provenanceIntegrityError("experience provenance URL must be a canonical https://github.com/ URL", ref)
		}
	}
	return experienceProvenance{URLs: urls}, nil
}

func persistExperienceRecords(root string, records []experienceRecord) ([]string, *CommandError) {
	shards, ingestErr := ingestdoc.PersistRecords(root, records)
	return shards, commandErrorFromIngest(ingestErr)
}

func redactForPersistence(event map[string]any, ref string) (any, *CommandError) {
	redacted, ingestErr := ingestdoc.RedactForPersistence(event, ref)
	return redacted, commandErrorFromIngest(ingestErr)
}

func validGitHubProvenanceURLShape(value string) bool {
	return ingestdoc.ValidGitHubProvenanceURLShape(value)
}

func gitHubProvenanceURLRepoMatchesExperience(value string, record experienceRecord) bool {
	return ingestdoc.GitHubProvenanceURLRepoMatchesRecord(value, record)
}

func gitHubPullRequestURLPathNumber(value string) (int, bool) {
	return ingestdoc.GitHubPullRequestURLPathNumber(value)
}

func gitHubPullRequestURLNumber(value string) (int, bool) {
	return ingestdoc.GitHubPullRequestURLNumber(value)
}

func gitHubPullRequestURLMatchesExperience(value string, record experienceRecord) bool {
	return ingestdoc.GitHubPullRequestURLMatchesRecord(value, record)
}

func gitHubPullRequestURLForExperience(record experienceRecord) string {
	return ingestdoc.GitHubPullRequestURLForRecord(record)
}

func nestedField(event map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = event
	for _, part := range parts {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapping[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func stringField(event map[string]any, paths ...string) string {
	for _, path := range paths {
		if value, ok := nestedField(event, path); ok {
			if converted := stringFromAny(value); converted != "" {
				return converted
			}
		}
	}
	return ""
}

func intField(event map[string]any, fallback int, paths ...string) int {
	for _, path := range paths {
		if value, ok := nestedField(event, path); ok {
			if converted, ok := intFromAny(value); ok {
				return converted
			}
		}
	}
	return fallback
}

func requiredPositiveIntField(event map[string]any, ref string, fieldDescription string, paths ...string) (int, *CommandError) {
	for _, path := range paths {
		value, ok := nestedField(event, path)
		if !ok {
			continue
		}
		converted, ok := intFromAny(value)
		if !ok || converted < 1 {
			return 0, provenanceIntegrityError(fieldDescription+" must be a positive integer", ref)
		}
		return converted, nil
	}
	return 0, provenanceIntegrityError(fieldDescription+" must be provided", ref)
}

func intFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) {
			return 0, false
		}
		return int64ToInt(int64(typed))
	case int:
		return typed, true
	case json.Number:
		converted, err := typed.Int64()
		if err == nil {
			return int64ToInt(converted)
		}
	case string:
		converted, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return converted, true
		}
	}
	return 0, false
}

func int64ToInt(value int64) (int, bool) {
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	if value < minInt || value > maxInt {
		return 0, false
	}
	return int(value), true
}

func floatField(event map[string]any, fallback float64, paths ...string) float64 {
	for _, path := range paths {
		if value, ok := nestedField(event, path); ok {
			if converted, ok := numericValue(value); ok {
				return converted
			}
		}
	}
	return fallback
}

func optionalFloatField(event map[string]any, ref string, label string, paths ...string) (float64, bool, *CommandError) {
	for _, path := range paths {
		if value, ok := nestedField(event, path); ok {
			converted, valid := numericValue(value)
			if !valid {
				return 0, true, artifactContractError(label+" must be numeric", ref)
			}
			return converted, true, nil
		}
	}
	return 0, false, nil
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return typed, true
	case int:
		return float64(typed), true
	case json.Number:
		converted, err := typed.Float64()
		if err == nil && !math.IsNaN(converted) && !math.IsInf(converted, 0) {
			return converted, true
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, false
		}
		converted, err := strconv.ParseFloat(trimmed, 64)
		if err == nil && !math.IsNaN(converted) && !math.IsInf(converted, 0) {
			return converted, true
		}
	}
	return 0, false
}

func stringListField(event map[string]any, paths ...string) []string {
	for _, path := range paths {
		if value, ok := nestedField(event, path); ok {
			switch typed := value.(type) {
			case []any:
				var result []string
				for _, item := range typed {
					if converted := stringFromAny(item); converted != "" {
						result = append(result, converted)
					}
				}
				if len(result) > 0 {
					return result
				}
			case []string:
				if len(typed) > 0 {
					return typed
				}
			case string:
				if strings.TrimSpace(typed) != "" {
					return []string{strings.TrimSpace(typed)}
				}
			}
		}
	}
	return nil
}

func metadataField(event map[string]any) map[string]any {
	metadata := map[string]any{}
	if value, ok := nestedField(event, "metadata"); ok {
		if source, ok := value.(map[string]any); ok {
			for key, item := range source {
				metadata[key] = item
			}
		}
	}
	return metadata
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func yamlListValues(document yamlDocument, path string) []string {
	return yamlmini.ListValues(document, path)
}

func yamlListValuesWithMapFields(document yamlDocument, path string, fields ...string) []string {
	return yamlmini.ListValuesWithMapFields(document, path, fields...)
}

func overlaps(left []string, right []string) bool {
	for _, candidate := range left {
		if containsStringValue(right, candidate) {
			return true
		}
	}
	return false
}

func containsStringValue(values []string, want string) bool {
	want = strings.TrimSpace(strings.ToLower(want))
	if want == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(strings.ToLower(value)) == want {
			return true
		}
	}
	return false
}

func defaultAttributionConfidence(method string) float64 {
	switch method {
	case "manual":
		return 1
	case "pr_label", "coauthor_trailer", "bot_login":
		return 0.9
	default:
		return 0
	}
}

func validOutcomeKind(kind string) bool {
	switch kind {
	case "merged_clean", "ci_failure", "revert", "review_correction", "fix_held":
		return true
	default:
		return false
	}
}

func terminalStateForOutcome(kind string) string {
	switch kind {
	case "merged_clean":
		return "passed"
	case "ci_failure":
		return "failed"
	case "revert":
		return "reverted"
	case "review_correction":
		return "corrected"
	case "fix_held":
		return "held"
	default:
		return ""
	}
}

func validTerminalState(value string) bool {
	switch value {
	case "passed", "failed", "reverted", "corrected", "held":
		return true
	default:
		return false
	}
}

func signatureClassForOutcome(kind string) string {
	switch kind {
	case "revert":
		return "revert"
	case "review_correction":
		return "review_correction"
	case "ci_failure":
		return "test_failure"
	default:
		return "unknown"
	}
}

func validExtractionConfidence(value string) bool {
	switch value {
	case "structured", "log_parsed_high", "log_parsed_low", "unknown":
		return true
	default:
		return false
	}
}

func extractionMethodForConfidence(value string) string {
	switch value {
	case "log_parsed_high", "log_parsed_low":
		return "log_parse"
	case "unknown":
		return "revert_metadata"
	default:
		return "structured_check_run"
	}
}

func sha256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + fmt.Sprintf("%x", digest)
}

func shortHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest)[:12]
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueInts(values []int) []int {
	seen := map[int]struct{}{}
	var result []int
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func displayPath(root string, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Join("external-input", filepath.Base(path)))
}

func helpResult(start time.Time) CommandResult {
	return passResult("help", "help", "relia command surface", start, map[string]any{
		"primary_commands": primaryCommands,
		"auxiliary_commands": []string{
			"models pull",
			"demo",
			"share",
		},
		"global_flags": []string{
			"--json",
			"--quiet",
			"--compact",
			"--help",
			"--version",
		},
	})
}

func passResult(command string, mode string, message string, start time.Time, data map[string]any) CommandResult {
	return resultdoc.Pass(command, mode, message, start, data, commandResultBuildOptions())
}

func notImplementedResult(command string, start time.Time) CommandResult {
	return errorResult(command, command, &CommandError{
		Type:        "not_implemented",
		Message:     command + " is reserved by the MVP command model but not implemented in this task slice",
		ExitCode:    ExitInternal,
		Remediation: "Use relia init and relia check for the T1 lifecycle baseline; later task packets implement this command.",
		Ref:         "docs/product/prd.md#command-model",
	}, start)
}

func errorResult(command string, mode string, commandErr *CommandError, start time.Time) CommandResult {
	return resultdoc.Error(command, mode, commandErr, start, commandResultBuildOptions())
}

func errorResultWithData(command string, mode string, commandErr *CommandError, start time.Time, data map[string]any) CommandResult {
	return resultdoc.ErrorWithData(command, mode, commandErr, start, data, commandResultBuildOptions())
}

func commandResultBuildOptions() resultdoc.BuildOptions {
	return resultdoc.BuildOptions{
		SchemaVersion:           commandSchemaVersion,
		ReliaVersion:            reliaVersion,
		SuccessExitCode:         ExitSuccess,
		RedactionSafetyExitCode: ExitRedactionSafety,
	}
}

func assessmentBuildOptions() assessdoc.Options {
	return assessdoc.Options{
		SchemaVersion:            commandSchemaVersion,
		ArtifactContractError:    artifactContractError,
		InternalError:            internalError,
		ProvenanceIntegrityError: provenanceIntegrityError,
		RepoPathExists:           configdoc.RepoPathExists,
		YAMLFloat:                yamlFloat,
	}
}

func usageError(message string) *CommandError {
	return &CommandError{
		Type:        "invalid_usage",
		Message:     message,
		ExitCode:    ExitUsage,
		Remediation: "Run relia --help for supported commands and flags.",
		Ref:         "docs/product/prd.md#command-model",
	}
}

func configError(message string) *CommandError {
	return &CommandError{
		Type:        "local_configuration_error",
		Message:     message,
		ExitCode:    ExitUsage,
		Remediation: "Run relia init from the repository root and then relia check.",
		Ref:         defaultConfigFile,
	}
}

func configErrorAt(message string, ref string) *CommandError {
	commandErr := configError(message)
	commandErr.Ref = ref
	return commandErr
}

func commandErrorFromConfig(configErr *configdoc.Error) *CommandError {
	if configErr == nil {
		return nil
	}
	switch configErr.Kind {
	case configdoc.ErrorConfig:
		if configErr.Ref == "" {
			return configError(configErr.Message)
		}
		return configErrorAt(configErr.Message, configErr.Ref)
	case configdoc.ErrorArtifactContract:
		return artifactContractError(configErr.Message, configErr.Ref)
	case configdoc.ErrorRedactionSafety:
		return redactionSafetyError(configErr.Message, configErr.Ref)
	case configdoc.ErrorDependency:
		return dependencyError(configErr.Message, configErr.Ref)
	case configdoc.ErrorInternal:
		return internalError(configErr.Message, configErr.Err)
	default:
		return internalError(configErr.Message, configErr.Err)
	}
}

func commandErrorFromIngest(ingestErr *ingestdoc.Error) *CommandError {
	if ingestErr == nil {
		return nil
	}
	switch ingestErr.Kind {
	case ingestdoc.ErrorArtifactContract:
		return artifactContractError(ingestErr.Message, ingestErr.Ref)
	case ingestdoc.ErrorInternal:
		return internalError(ingestErr.Message, nil)
	case ingestdoc.ErrorProvenance:
		return provenanceIntegrityError(ingestErr.Message, ingestErr.Ref)
	case ingestdoc.ErrorRedactionSafety:
		return redactionSafetyError(ingestErr.Message, ingestErr.Ref)
	default:
		return internalError(ingestErr.Message, nil)
	}
}

func validationError(message string, missing []string) *CommandError {
	return &CommandError{
		Type:        "operating_pack_validation_failed",
		Message:     message + ": " + strings.Join(missing, ", "),
		ExitCode:    ExitValidation,
		Remediation: "Restore the required repo lifecycle files before running Relia workflows.",
		Ref:         "docs/dev/dev_guides.md#validation-matrix",
	}
}

func artifactContractError(message string, ref string) *CommandError {
	return &CommandError{
		Type:        "artifact_contract_validation_failed",
		Message:     message,
		ExitCode:    ExitValidation,
		Remediation: "Repair the schema, config, or memory artifact so it matches the versioned Relia contract.",
		Ref:         ref,
	}
}

func redactionSafetyError(message string, ref string) *CommandError {
	return &CommandError{
		Type:        "redaction_safety_failed",
		Message:     message,
		ExitCode:    ExitRedactionSafety,
		Remediation: "Keep local-only privacy and fail-closed redaction enabled before persisting or sharing artifacts.",
		Ref:         ref,
	}
}

func provenanceIntegrityError(message string, ref string) *CommandError {
	return &CommandError{
		Type:        "provenance_integrity_failed",
		Message:     message,
		ExitCode:    ExitProvenanceIntegrity,
		Remediation: "Provide PR and source evidence URLs before persisting canonical experience records.",
		Ref:         ref,
	}
}

func dependencyError(message string, ref string) *CommandError {
	return &CommandError{
		Type:        "dependency_error",
		Message:     message,
		ExitCode:    ExitDependency,
		Remediation: "Run relia models pull with an approved model_artifact_pull gate or use embeddings: signature.",
		Ref:         ref,
	}
}

func internalError(message string, err error) *CommandError {
	if err != nil {
		message += ": " + err.Error()
	}
	return &CommandError{
		Type:        "internal_error",
		Message:     message,
		ExitCode:    ExitInternal,
		Remediation: "Rerun with --json and include the command result envelope in the task evidence.",
	}
}

func renderAndExit(stdout io.Writer, stderr io.Writer, result CommandResult, flags globalFlags, stdoutIsTTY bool) int {
	machineReadable := flags.json || flags.quiet || flags.compact || result.MachineReadable || !stdoutIsTTY
	var err error
	if machineReadable {
		err = writeJSON(stdout, result, flags.compact || flags.quiet)
	} else {
		err = writeHuman(stdout, stderr, result)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "relia: failed to render command result: %v\n", err)
		return ExitInternal
	}
	return result.ExitCode
}

func writeJSON(stdout io.Writer, result CommandResult, compact bool) error {
	encoder := json.NewEncoder(stdout)
	if !compact {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(result)
}

func writeHuman(stdout io.Writer, stderr io.Writer, result CommandResult) error {
	message, _ := result.Data["message"].(string)
	if message == "" && len(result.Errors) > 0 {
		message = result.Errors[0].Message
	}
	writer := stdout
	if result.ExitCode != ExitSuccess {
		writer = stderr
	}
	if _, err := fmt.Fprintf(writer, "%s %s: %s\n", result.Status, result.Command, message); err != nil {
		return err
	}
	if result.Command == "backtest" {
		report, ok := result.Data["report"].(recurrenceReport)
		if ok {
			reportPath, _ := result.Data["report_path"].(string)
			if reportPath == "" {
				reportPath, _ = result.Data["html_report_path"].(string)
			}
			if err := backtestdoc.WriteHumanDetails(writer, report, reportPath); err != nil {
				return err
			}
		}
	}
	if len(result.EvidenceRefs) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(writer, "evidence: %s\n", strings.Join(result.EvidenceRefs, ", "))
	return err
}

func stdoutIsTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func validateReliaConfig(root string) ([]Finding, *CommandError) {
	return validateReliaConfigWithEmbeddingOverride(root, "")
}

func validateReliaConfigForDistill(root string, embeddingOverride string) ([]Finding, *CommandError) {
	return validateReliaConfigWithEmbeddingOverride(root, embeddingOverride)
}

func validateReliaConfigWithEmbeddingOverride(root string, embeddingOverride string) ([]Finding, *CommandError) {
	warnings, configErr := configdoc.Validate(root, configdoc.ValidationOptions{
		SchemaVersion:     commandSchemaVersion,
		ReliaVersion:      reliaVersion,
		EmbeddingOverride: embeddingOverride,
	})
	return warnings, commandErrorFromConfig(configErr)
}

func validateAdviseConfig(document yamlDocument) *CommandError {
	_, commandErr := adviseSettingsFromConfig(document)
	return commandErr
}

func adviseSettingsFromConfig(document yamlDocument) (adviseSettings, *CommandError) {
	settings, configErr := configdoc.AdviseSettingsFromConfig(document)
	return settings, commandErrorFromConfig(configErr)
}

func validateLocalModelManifest(root string, manifestScalar yamlScalar) *CommandError {
	return commandErrorFromConfig(configdoc.ValidateLocalModelManifest(root, manifestScalar))
}

func validateLocalModelManifestPayload(root string, manifest configdoc.LocalModelManifest, ref string) *CommandError {
	return commandErrorFromConfig(configdoc.ValidateLocalModelManifestPayload(root, manifest, ref))
}

func validateSchemaContracts(root string) *CommandError {
	return commandErrorFromConfig(configdoc.ValidateSchemaContracts(root, requiredSchemaFiles))
}

func validateMemoryRuleArtifacts(root string) *CommandError {
	return memorydoc.ValidateRuleArtifacts(root, memoryValidationOptions())
}

func validateMemoryRuleArtifact(root string, path string) *CommandError {
	return memorydoc.ValidateRuleArtifact(root, path, memoryValidationOptions())
}

func validateDraftedMemoryRuleCalibration(document yamlDocument, rel string, confidence float64, evidenceCount int, contradictions int) *CommandError {
	return memorydoc.ValidateDraftedRuleCalibration(document, rel, confidence, evidenceCount, contradictions, memoryValidationOptions())
}

func reviewUpdateOptions() reviewdoc.UpdateOptions {
	return reviewdoc.UpdateOptions{
		SchemaVersion:         commandSchemaVersion,
		UsageError:            usageError,
		ArtifactContractError: artifactContractError,
		InternalError:         internalError,
		RepoPathExists:        configdoc.RepoPathExists,
		YAMLFloat:             yamlFloat,
	}
}

func memoryValidationOptions() memorydoc.ValidationOptions {
	return memorydoc.ValidationOptions{
		SchemaVersion:         commandSchemaVersion,
		ArtifactContractError: artifactContractError,
		InternalError:         internalError,
		RepoPathExists:        configdoc.RepoPathExists,
		YAMLFloat:             yamlFloat,
	}
}

func parseYAMLDocument(content string) (yamlDocument, error) {
	return yamlmini.ParseDocument(content)
}

func hasYAMLPath(document yamlDocument, path string) bool {
	return yamlmini.HasPath(document, path)
}

func leadingSpaces(value string) int {
	return yamlmini.LeadingSpaces(value)
}

func configRef(scalar yamlScalar) string {
	return configdoc.Ref(defaultConfigFile, scalar)
}

func configRefWithPath(path string, scalar yamlScalar) string {
	return configdoc.RefWithPath(path, scalar)
}

func yamlPathRef(document yamlDocument, path string) string {
	return configdoc.PathRef(defaultConfigFile, document, path)
}

func defaultConfigYAML() string {
	return configdoc.DefaultYAML(commandSchemaVersion, reliaVersion)
}
