package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
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
	commandResultObjectType = "relia.command_result"
	commandSchemaVersion    = "1.0"
	reliaVersion            = "0.0.0-dev"
	defaultConfigFile       = "relia.yaml"
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

var artifactSkeletonDirs = []string{
	".relia/experiences",
	".relia/signatures",
	".relia/coverage",
	".relia/reports",
	".relia/baselines",
	"memory/rules",
	"memory/compiled",
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
}

var knownSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`(?i)\bgh[opusr]_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{20,}\b`),
}

var unifiedHunkHeaderPattern = regexp.MustCompile(`^@@ -\d+(?:,(\d+))? \+\d+(?:,(\d+))? @@`)

type CommandResult struct {
	ObjectType      string         `json:"object_type"`
	SchemaVersion   string         `json:"schema_version"`
	Command         string         `json:"command"`
	Status          string         `json:"status"`
	Mode            string         `json:"mode"`
	ExitCode        int            `json:"exit_code"`
	Warnings        []Finding      `json:"warnings"`
	Errors          []CommandError `json:"errors"`
	Artifacts       []ArtifactRef  `json:"artifacts"`
	EvidenceRefs    []string       `json:"evidence_refs"`
	DurationMS      int64          `json:"duration_ms"`
	RedactionStatus string         `json:"redaction_status"`
	Metadata        map[string]any `json:"metadata"`
	Data            map[string]any `json:"data,omitempty"`
	MachineReadable bool           `json:"-"`
}

type Finding struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Ref     string `json:"ref,omitempty"`
}

type CommandError struct {
	Type        string `json:"type"`
	Message     string `json:"message"`
	ExitCode    int    `json:"exit_code"`
	Remediation string `json:"remediation,omitempty"`
	Ref         string `json:"ref,omitempty"`
}

type ArtifactRef struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type yamlScalar struct {
	Value string
	Line  int
}

type yamlDocument struct {
	Scalars    map[string]yamlScalar
	Lists      map[string][]yamlScalar
	ListMaps   map[string][]map[string]yamlScalar
	Containers map[string]yamlScalar
}

type yamlContext struct {
	Path       string
	ListItem   bool
	ListParent string
	ListIndex  int
}

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

type ingestOptions struct {
	InputPath string
}

type backtestOptions struct {
	Window       string
	Format       string
	BaselinePath string
	ReportDir    string
	SaveBaseline bool
}

type assessOptions struct {
	InputPath      string
	Format         string
	FormatExplicit bool
}

type riskAssessment struct {
	ObjectType    string                `json:"object_type"`
	SchemaVersion string                `json:"schema_version"`
	AssessmentID  string                `json:"assessment_id"`
	RiskLevel     string                `json:"risk_level"`
	Matches       []riskAssessmentMatch `json:"matches"`
	Citations     []string              `json:"citations"`
	Metadata      map[string]any        `json:"metadata"`
}

type riskAssessmentMatch struct {
	RuleID     string  `json:"rule_id"`
	Confidence float64 `json:"confidence"`
}

type assessmentRule struct {
	ID         string
	Kind       string
	Path       string
	Confidence float64
	ScopePaths []string
	Citations  []assessmentRuleCitation
}

type assessmentRuleCitation struct {
	URL     string
	PR      int
	Outcome string
}

type experienceRecord struct {
	ObjectType      string                `json:"object_type"`
	SchemaVersion   string                `json:"schema_version"`
	ExperienceID    string                `json:"experience_id"`
	Repo            experienceRepo        `json:"repo"`
	RecordedAt      string                `json:"recorded_at"`
	Attribution     experienceAttribution `json:"attribution"`
	Context         experienceContext     `json:"context"`
	Action          experienceAction      `json:"action"`
	Outcome         experienceOutcome     `json:"outcome"`
	Provenance      experienceProvenance  `json:"provenance"`
	FlakeDiscount   float64               `json:"flake_discount"`
	OrgEligible     bool                  `json:"org_eligible"`
	ShareScope      string                `json:"share_scope"`
	RedactionStatus string                `json:"redaction_status"`
	Metadata        map[string]any        `json:"metadata"`
}

type experienceRepo struct {
	Provider string `json:"provider"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
}

type experienceAttribution struct {
	ActorKind  string  `json:"actor_kind"`
	Method     string  `json:"method"`
	Confidence float64 `json:"confidence"`
}

type experienceContext struct {
	Paths           []string `json:"paths"`
	DiffFingerprint string   `json:"diff_fingerprint"`
}

type experienceAction struct {
	PR     int    `json:"pr"`
	Commit string `json:"commit"`
}

type experienceOutcome struct {
	Kind          string              `json:"kind"`
	TerminalState string              `json:"terminal_state"`
	Signature     experienceSignature `json:"signature"`
}

type experienceSignature struct {
	SignatureID          string `json:"signature_id"`
	ExtractionConfidence string `json:"extraction_confidence"`
}

type experienceProvenance struct {
	URLs []string `json:"urls"`
}

type backtestExperience struct {
	Record     experienceRecord
	RecordedAt time.Time
	SourcePath string
	SourceLine int
}

type recurrenceReport struct {
	ObjectType           string                  `json:"object_type"`
	SchemaVersion        string                  `json:"schema_version"`
	ReportID             string                  `json:"report_id"`
	SourceArtifacts      []string                `json:"source_artifacts"`
	Window               recurrenceWindow        `json:"window"`
	Summary              recurrenceSummary       `json:"summary"`
	HeadlineERR          float64                 `json:"headline_err"`
	ConfirmedRecurrences []recurrencePair        `json:"confirmed_recurrences"`
	PossibleRecurrences  []recurrencePair        `json:"possible_recurrences"`
	FlakeDiscounts       []backtestFlakeDiscount `json:"flake_discounts"`
	AttributionUncertain []backtestUncertain     `json:"attribution_uncertain"`
	Baseline             baselineComparison      `json:"baseline"`
	Gate                 backtestGateResult      `json:"gate"`
	Citations            []backtestCitation      `json:"citations"`
	Metadata             map[string]any          `json:"metadata"`
}

type recurrenceWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type recurrenceSummary struct {
	ExperienceCount           int     `json:"experience_count"`
	WindowExperienceCount     int     `json:"window_experience_count"`
	AgentFailureDenominator   int     `json:"agent_failure_denominator"`
	ConfirmedRecurrenceCount  int     `json:"confirmed_recurrence_count"`
	PossibleRecurrenceCount   int     `json:"possible_recurrence_count"`
	HeadlineERR               float64 `json:"headline_err"`
	HeadlineERRPercent        string  `json:"headline_err_percent"`
	FlakeDiscountedCount      int     `json:"flake_discounted_count"`
	AttributionUncertainCount int     `json:"attribution_uncertain_count"`
	HumanFailureExcludedCount int     `json:"human_failure_excluded_count"`
	NonFailureOutcomeCount    int     `json:"non_failure_outcome_count"`
}

type recurrencePair struct {
	CurrentExperienceID string   `json:"current_experience_id"`
	PriorExperienceID   string   `json:"prior_experience_id"`
	CurrentPR           int      `json:"current_pr"`
	PriorPR             int      `json:"prior_pr"`
	CurrentURL          string   `json:"current_url"`
	PriorURL            string   `json:"prior_url"`
	SignatureID         string   `json:"signature_id"`
	Confidence          string   `json:"confidence"`
	Reason              string   `json:"reason"`
	Refs                []string `json:"refs"`
}

type backtestFlakeDiscount struct {
	ExperienceID    string   `json:"experience_id"`
	PR              int      `json:"pr"`
	SignatureID     string   `json:"signature_id"`
	FlakeDiscount   float64  `json:"flake_discount"`
	SupportingPRs   []int    `json:"supporting_prs"`
	SupportingRefs  []string `json:"supporting_refs"`
	Reason          string   `json:"reason"`
	ExcludedFromERR bool     `json:"excluded_from_err"`
}

type backtestUncertain struct {
	ExperienceID          string  `json:"experience_id"`
	PR                    int     `json:"pr"`
	OutcomeKind           string  `json:"outcome_kind"`
	AttributionMethod     string  `json:"attribution_method"`
	AttributionConfidence float64 `json:"attribution_confidence"`
	ExcludedFromERR       bool    `json:"excluded_from_err"`
	Ref                   string  `json:"ref"`
	Reason                string  `json:"reason"`
}

type baselineComparison struct {
	Status      string  `json:"status"`
	Path        string  `json:"path"`
	HeadlineERR float64 `json:"headline_err,omitempty"`
	Delta       float64 `json:"delta,omitempty"`
	Stale       bool    `json:"stale"`
	Reason      string  `json:"reason"`
}

type backtestGateResult struct {
	Enabled   bool     `json:"enabled"`
	Status    string   `json:"status"`
	Threshold *float64 `json:"threshold,omitempty"`
	Reason    string   `json:"reason"`
	Ref       string   `json:"ref"`
}

type backtestCitation struct {
	PR           int    `json:"pr"`
	URL          string `json:"url"`
	ExperienceID string `json:"experience_id"`
}

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
	case "models":
		if len(parsed.commandArgs) == 1 && parsed.commandArgs[0] == "pull" {
			return notImplementedResult("models pull", start)
		}
		return errorResult("models", "models", usageError("expected subcommand: pull"), start)
	case "distill", "review", "memory", "compile", "serve", "demo", "share":
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
	root, ok := findRepoRoot(wd)
	if !ok {
		root = wd
	}
	configPath := filepath.Join(root, defaultConfigFile)
	artifact := ArtifactRef{Kind: "config", Path: defaultConfigFile}
	if _, err := os.Stat(configPath); err == nil {
		if err := ensureArtifactSkeleton(root); err != nil {
			return errorResult("init", "init", internalError("could not write artifact skeleton", err), start)
		}
		if err := ensureReliaGitIgnore(root); err != nil {
			return errorResult("init", "init", internalError("could not update .gitignore", err), start)
		}
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
	if err := ensureArtifactSkeleton(root); err != nil {
		return errorResult("init", "init", internalError("could not write artifact skeleton", err), start)
	}
	if err := ensureReliaGitIgnore(root); err != nil {
		return errorResult("init", "init", internalError("could not update .gitignore", err), start)
	}
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
	root, ok := findRepoRoot(wd)
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
	options, commandErr := parseIngestArgs(args)
	if commandErr != nil {
		return errorResult("ingest", "ingest", commandErr, start)
	}
	wd, err := os.Getwd()
	if err != nil {
		return errorResult("ingest", "ingest", internalError("could not inspect working directory", err), start)
	}
	root, ok := findRepoRoot(wd)
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

func parseIngestArgs(args []string) (ingestOptions, *CommandError) {
	var options ingestOptions
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--input", "-i":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("ingest requires a path after --input")
			}
			options.InputPath = args[index+1]
			index++
		default:
			return options, usageError(fmt.Sprintf("unknown ingest argument %q", arg))
		}
	}
	if strings.TrimSpace(options.InputPath) == "" {
		return options, usageError("ingest requires --input <json-or-jsonl> in offline mode")
	}
	return options, nil
}

func backtestResult(args []string, start time.Time) CommandResult {
	options, commandErr := parseBacktestArgs(args)
	withFormat := func(result CommandResult) CommandResult {
		if options.Format == "json" {
			result.MachineReadable = true
		}
		return result
	}
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}
	wd, err := os.Getwd()
	if err != nil {
		return withFormat(errorResult("backtest", "backtest", internalError("could not inspect working directory", err), start))
	}
	root, ok := findRepoRoot(wd)
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
	windowDays, commandErr := parseBacktestWindowDays(options.Window)
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}
	report, commandErr := buildRecurrenceReport(root, config, records, sourceArtifacts, sourceDigest, options, windowDays)
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}
	if options.SaveBaseline {
		if commandErr := saveBacktestBaseline(root, report, options.BaselinePath); commandErr != nil {
			return withFormat(errorResult("backtest", "backtest", commandErr, start))
		}
		report.Baseline, commandErr = savedBacktestBaselineComparison(options.BaselinePath, report.HeadlineERR)
		if commandErr != nil {
			return withFormat(errorResult("backtest", "backtest", commandErr, start))
		}
	}
	jsonReportPath, htmlReportPath, commandErr := writeBacktestReports(root, report, options.ReportDir)
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}

	result := passResult("backtest", "backtest", "computed conservative recurrence backtest", start, map[string]any{
		"window":                 report.Window,
		"headline_err":           report.HeadlineERR,
		"headline_err_percent":   report.Summary.HeadlineERRPercent,
		"confirmed_recurrences":  report.Summary.ConfirmedRecurrenceCount,
		"possible_recurrences":   report.Summary.PossibleRecurrenceCount,
		"flake_discounted_count": report.Summary.FlakeDiscountedCount,
		"baseline":               report.Baseline,
		"gate":                   report.Gate,
		"report":                 report,
		"json_report_path":       jsonReportPath,
		"html_report_path":       htmlReportPath,
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
			Message:     fmt.Sprintf("headline ERR %.4f exceeds configured gate %.4f", report.HeadlineERR, backtestGateThresholdValue(report.Gate)),
			ExitCode:    ExitGate,
			Remediation: "Leave gate.enabled false for advisory-only MVP behavior or raise the explicit threshold after reviewing the recurrence report.",
			Ref:         report.Gate.Ref,
		})
	}
	return withFormat(result)
}

func parseBacktestArgs(args []string) (backtestOptions, *CommandError) {
	options := backtestOptions{
		Window:       "180d",
		Format:       "json",
		BaselinePath: ".relia/baselines/error-recurrence-baseline.json",
		ReportDir:    ".relia/reports",
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--window":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("backtest requires a value after --window")
			}
			options.Window = args[index+1]
			index++
		case "--format":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("backtest requires a value after --format")
			}
			options.Format = args[index+1]
			index++
		case "--baseline":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("backtest requires a repo-relative path after --baseline")
			}
			options.BaselinePath = args[index+1]
			index++
		case "--report-dir":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("backtest requires a repo-relative path after --report-dir")
			}
			options.ReportDir = args[index+1]
			index++
		case "--save-baseline":
			options.SaveBaseline = true
		default:
			return options, usageError(fmt.Sprintf("unknown backtest argument %q", arg))
		}
	}
	if options.Format != "json" {
		return options, usageError("backtest only supports --format json in this task slice")
	}
	if _, commandErr := parseBacktestWindowDays(options.Window); commandErr != nil {
		return options, commandErr
	}
	if _, ok := cleanRepoPath(options.BaselinePath); !ok {
		return options, usageError("backtest --baseline must be a repo-relative path")
	}
	if _, ok := cleanRepoPath(options.ReportDir); !ok {
		return options, usageError("backtest --report-dir must be a repo-relative path")
	}
	return options, nil
}

func parseBacktestWindowDays(value string) (int, *CommandError) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if !strings.HasSuffix(trimmed, "d") {
		return 0, usageError("backtest --window must use a day duration such as 180d")
	}
	days, err := strconv.Atoi(strings.TrimSuffix(trimmed, "d"))
	if err != nil || days <= 0 {
		return 0, usageError("backtest --window must be a positive day duration such as 180d")
	}
	return days, nil
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
		digestParts = append(digestParts, rel+"\x00"+sha256String(string(content)))
		for lineNumber, line := range strings.Split(string(content), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var record experienceRecord
			if err := decodeJSONUseNumber(line, &record); err != nil {
				return nil, nil, "", artifactContractError(fmt.Sprintf("experience shard line %d is not valid JSON", lineNumber+1), fmt.Sprintf("%s:%d", rel, lineNumber+1))
			}
			recordedAt, commandErr := validateBacktestExperience(record, fmt.Sprintf("%s:%d", rel, lineNumber+1))
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
	}
	if len(records) == 0 {
		return nil, nil, "", artifactContractError("backtest found no experience records in .relia/experiences", ".relia/experiences")
	}
	sort.Strings(digestParts)
	return records, sourceArtifacts, sha256String(strings.Join(digestParts, "\x00")), nil
}

func validateBacktestExperience(record experienceRecord, ref string) (time.Time, *CommandError) {
	if record.ObjectType != "relia.experience_record" {
		return time.Time{}, artifactContractError("backtest experience object_type must be relia.experience_record", ref)
	}
	if record.SchemaVersion != commandSchemaVersion {
		return time.Time{}, artifactContractError("backtest experience schema_version must be "+commandSchemaVersion, ref)
	}
	if strings.TrimSpace(record.ExperienceID) == "" {
		return time.Time{}, artifactContractError("backtest experience missing experience_id", ref)
	}
	recordedAt, err := time.Parse(time.RFC3339, record.RecordedAt)
	if err != nil {
		return time.Time{}, artifactContractError("backtest experience recorded_at must be RFC3339", ref)
	}
	if record.Repo.Provider != "github" || record.Repo.Owner == "" || record.Repo.Name == "" {
		return time.Time{}, artifactContractError("backtest experience repo must include github owner and name", ref)
	}
	if record.Action.PR < 1 {
		return time.Time{}, provenanceIntegrityError("backtest experience action.pr must be a positive integer", ref)
	}
	if !validOutcomeKind(record.Outcome.Kind) || !validTerminalState(record.Outcome.TerminalState) {
		return time.Time{}, artifactContractError("backtest experience outcome is invalid", ref)
	}
	if strings.TrimSpace(record.Outcome.Signature.SignatureID) == "" {
		return time.Time{}, artifactContractError("backtest experience outcome.signature.signature_id must be provided", ref)
	}
	if !validExtractionConfidence(record.Outcome.Signature.ExtractionConfidence) {
		return time.Time{}, artifactContractError("backtest experience signature extraction_confidence is invalid", ref)
	}
	switch record.Attribution.ActorKind {
	case "agent", "human", "uncertain":
	default:
		return time.Time{}, artifactContractError("backtest experience attribution actor_kind must be agent, human, or uncertain", ref)
	}
	if len(record.Context.Paths) == 0 {
		return time.Time{}, artifactContractError("backtest experience context.paths must include at least one path", ref)
	}
	if record.FlakeDiscount < 0 || record.FlakeDiscount > 1 || math.IsNaN(record.FlakeDiscount) || math.IsInf(record.FlakeDiscount, 0) {
		return time.Time{}, artifactContractError("backtest experience flake_discount must be between 0 and 1", ref)
	}
	if len(record.Provenance.URLs) == 0 {
		return time.Time{}, provenanceIntegrityError("backtest experience must include provenance URLs", ref)
	}
	for _, value := range record.Provenance.URLs {
		if !strings.HasPrefix(value, "https://github.com/") {
			return time.Time{}, provenanceIntegrityError("backtest experience provenance URL must be an https://github.com/ URL", ref)
		}
	}
	return recordedAt.UTC(), nil
}

func buildRecurrenceReport(root string, config yamlDocument, records []backtestExperience, sourceArtifacts []string, sourceDigest string, options backtestOptions, windowDays int) (recurrenceReport, *CommandError) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].RecordedAt.Equal(records[j].RecordedAt) {
			return records[i].Record.ExperienceID < records[j].Record.ExperienceID
		}
		return records[i].RecordedAt.Before(records[j].RecordedAt)
	})
	windowEnd := records[len(records)-1].RecordedAt.UTC()
	windowStart := windowEnd.AddDate(0, 0, -windowDays)
	windowRecords := make([]backtestExperience, 0, len(records))
	for _, record := range records {
		if record.RecordedAt.Before(windowStart) || record.RecordedAt.After(windowEnd) {
			continue
		}
		windowRecords = append(windowRecords, record)
	}
	flakeHeuristics := autoFlakeDiscountedExperiences(windowRecords)
	priorBySignature := map[string][]backtestExperience{}
	var confirmed []recurrencePair
	var possible []recurrencePair
	var flakes []backtestFlakeDiscount
	var uncertain []backtestUncertain
	citationMap := map[int]backtestCitation{}
	summary := recurrenceSummary{
		ExperienceCount:       len(records),
		WindowExperienceCount: len(windowRecords),
	}
	for _, current := range windowRecords {
		record := current.Record
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
		if record.Attribution.ActorKind != "agent" {
			summary.HumanFailureExcludedCount++
			continue
		}
		if isBacktestFlakeDiscounted(current, flakeHeuristics) {
			summary.FlakeDiscountedCount++
			flakes = append(flakes, buildBacktestFlakeDiscount(current, windowRecords, flakeHeuristics))
			addBacktestCitation(citationMap, current)
			continue
		}
		summary.AgentFailureDenominator++
		signatureID := record.Outcome.Signature.SignatureID
		if priors := priorBySignature[signatureID]; len(priors) > 0 {
			prior, confidence := selectRecurrencePrior(priors, current)
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
		priorBySignature[signatureID] = append(priorBySignature[signatureID], current)
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
	window := recurrenceWindow{
		Start: windowStart.Format(time.RFC3339),
		End:   windowEnd.Format(time.RFC3339),
	}
	baseline, commandErr := compareBacktestBaseline(root, options.BaselinePath, summary.HeadlineERR, sourceDigest, window)
	if commandErr != nil {
		return recurrenceReport{}, commandErr
	}
	gate := backtestGate(config, summary.HeadlineERR)
	report := recurrenceReport{
		ObjectType:           "relia.recurrence_report",
		SchemaVersion:        commandSchemaVersion,
		SourceArtifacts:      append([]string(nil), sourceArtifacts...),
		Window:               window,
		Summary:              summary,
		HeadlineERR:          summary.HeadlineERR,
		ConfirmedRecurrences: confirmed,
		PossibleRecurrences:  possible,
		FlakeDiscounts:       flakes,
		AttributionUncertain: uncertain,
		Baseline:             baseline,
		Gate:                 gate,
		Citations:            citations,
		Metadata: map[string]any{
			"window_days":                   windowDays,
			"source_artifact_digest":        sourceDigest,
			"possible_excluded_from_err":    true,
			"flake_discount_excluded":       true,
			"deterministic_window_anchor":   "latest_recorded_at_in_source_artifacts",
			"network_required":              false,
			"redaction_status":              "customer_safe",
			"repo_relative_paths_only":      true,
			"baseline_gate_enabled_default": false,
		},
	}
	report.ReportID = "backtest_" + shortHash(strings.Join([]string{
		report.Window.Start,
		report.Window.End,
		sourceDigest,
		strconv.Itoa(summary.AgentFailureDenominator),
		strconv.Itoa(summary.ConfirmedRecurrenceCount),
		strconv.Itoa(summary.PossibleRecurrenceCount),
	}, "\x00"))
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

func confirmedRecurrence(prior backtestExperience, current backtestExperience) bool {
	if prior.Record.Outcome.Signature.SignatureID == "" ||
		prior.Record.Outcome.Signature.SignatureID != current.Record.Outcome.Signature.SignatureID {
		return false
	}
	if !reliableSignatureExtraction(prior.Record.Outcome.Signature.ExtractionConfidence) ||
		!reliableSignatureExtraction(current.Record.Outcome.Signature.ExtractionConfidence) {
		return false
	}
	return pathSetsOverlap(prior.Record.Context.Paths, current.Record.Context.Paths)
}

func selectRecurrencePrior(priors []backtestExperience, current backtestExperience) (backtestExperience, string) {
	for index := len(priors) - 1; index >= 0; index-- {
		if confirmedRecurrence(priors[index], current) {
			return priors[index], "confirmed"
		}
	}
	return priors[len(priors)-1], "possible"
}

func pathSetsOverlap(left []string, right []string) bool {
	leftSet := map[string]bool{}
	for _, value := range left {
		if clean, ok := cleanRepoPath(value); ok {
			leftSet[filepath.ToSlash(clean)] = true
		}
	}
	for _, value := range right {
		if clean, ok := cleanRepoPath(value); ok && leftSet[filepath.ToSlash(clean)] {
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
		CurrentURL:          primaryProvenanceURL(current.Record),
		PriorURL:            primaryProvenanceURL(prior.Record),
		SignatureID:         current.Record.Outcome.Signature.SignatureID,
		Refs:                []string{sourceLineRef(prior), sourceLineRef(current)},
	}
}

func autoFlakeDiscountedExperiences(records []backtestExperience) map[string]string {
	bySignature := map[string][]backtestExperience{}
	for _, record := range records {
		if record.Record.Attribution.ActorKind != "agent" || !isFailureOutcome(record.Record.Outcome.Kind) {
			continue
		}
		if record.Record.Outcome.Signature.SignatureID == "" {
			continue
		}
		bySignature[record.Record.Outcome.Signature.SignatureID] = append(bySignature[record.Record.Outcome.Signature.SignatureID], record)
	}
	discounted := map[string]string{}
	for _, group := range bySignature {
		if len(group) < 3 || !groupHasUnrelatedNonTestDiffs(group) {
			continue
		}
		reason := "Discounted as flaky because the same failure signature appears at least three times across unrelated non-test diff paths."
		for _, record := range group {
			if record.Record.FlakeDiscount == 0 {
				discounted[record.Record.ExperienceID] = reason
			}
		}
	}
	return discounted
}

func groupHasUnrelatedNonTestDiffs(group []backtestExperience) bool {
	seen := map[string]bool{}
	for _, record := range group {
		paths := nonTestPaths(record.Record.Context.Paths)
		if len(paths) == 0 {
			paths = normalizedRepoPaths(record.Record.Context.Paths)
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

func nonTestPaths(paths []string) []string {
	var result []string
	for _, clean := range normalizedRepoPaths(paths) {
		base := path.Base(clean)
		if strings.HasPrefix(clean, "tests/") || strings.Contains(clean, "/tests/") || strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.go") {
			continue
		}
		result = append(result, clean)
	}
	return result
}

func normalizedRepoPaths(paths []string) []string {
	var result []string
	for _, value := range paths {
		if clean, ok := cleanRepoPath(value); ok {
			result = append(result, filepath.ToSlash(clean))
		}
	}
	result = uniqueStrings(result)
	sort.Strings(result)
	return result
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
		if candidate.Record.Outcome.Signature.SignatureID != record.Record.Outcome.Signature.SignatureID {
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
	url := primaryProvenanceURL(record.Record)
	if url == "" {
		return
	}
	citations[record.Record.Action.PR] = backtestCitation{
		PR:           record.Record.Action.PR,
		URL:          url,
		ExperienceID: record.Record.ExperienceID,
	}
}

func primaryProvenanceURL(record experienceRecord) string {
	for _, value := range record.Provenance.URLs {
		if prNumber, ok := gitHubPullRequestURLNumber(value); ok && prNumber == record.Action.PR {
			return value
		}
	}
	if len(record.Provenance.URLs) > 0 {
		return record.Provenance.URLs[0]
	}
	return ""
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
	clean, ok := cleanRepoPath(baselinePath)
	if !ok {
		return baselineComparison{}, usageError("backtest baseline path must be repo-relative")
	}
	rel := filepath.ToSlash(clean)
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return baselineComparison{
				Status: "missing",
				Path:   rel,
				Stale:  false,
				Reason: "No saved ERR baseline exists yet; use --save-baseline after reviewing the report to create one.",
			}, nil
		}
		return baselineComparison{}, internalError("could not read ERR baseline", err)
	}
	var payload map[string]any
	if err := decodeJSONUseNumber(string(content), &payload); err != nil {
		return baselineComparison{}, artifactContractError("ERR baseline is not valid JSON", rel)
	}
	baselineERR, ok := numericValue(payload["headline_err"])
	if !ok {
		if summary, summaryOK := payload["summary"].(map[string]any); summaryOK {
			if summaryERR, headlineOK := numericValue(summary["headline_err"]); headlineOK {
				baselineERR = summaryERR
				ok = true
			}
		}
	}
	if !ok || baselineERR < 0 || baselineERR > 1 {
		return baselineComparison{}, artifactContractError("ERR baseline missing numeric headline_err", rel)
	}
	baselineDigest := ""
	if metadata, ok := payload["metadata"].(map[string]any); ok {
		baselineDigest = stringFromAny(metadata["source_artifact_digest"])
	}
	status := "current"
	reason := "Saved baseline was computed from the same source artifact digest."
	stale := false
	if baselineDigest == "" || baselineDigest != sourceDigest {
		status = "stale"
		stale = true
		reason = "Saved baseline source artifact digest differs from the current backtest inputs."
	} else if !baselineWindowMatches(payload, window) {
		status = "stale"
		stale = true
		reason = "Saved baseline window differs from the current backtest window."
	}
	return baselineComparison{
		Status:      status,
		Path:        rel,
		HeadlineERR: roundFloat(baselineERR, 4),
		Delta:       roundFloat(headlineERR-baselineERR, 4),
		Stale:       stale,
		Reason:      reason,
	}, nil
}

func baselineWindowMatches(payload map[string]any, window recurrenceWindow) bool {
	baselineWindow, ok := payload["window"].(map[string]any)
	if !ok {
		return false
	}
	return stringFromAny(baselineWindow["start"]) == window.Start &&
		stringFromAny(baselineWindow["end"]) == window.End
}

func backtestGate(config yamlDocument, headlineERR float64) backtestGateResult {
	enabled := false
	ref := defaultConfigFile
	if scalar, ok := config.Scalars["gate.enabled"]; ok {
		enabled = scalar.Value == "true"
		ref = configRef(scalar)
	}
	if !enabled {
		return backtestGateResult{
			Enabled: false,
			Status:  "off",
			Reason:  "Recurrence gate is available but disabled by default for advisory-only MVP behavior.",
			Ref:     ref,
		}
	}
	threshold := 1.0
	if scalar, ok := config.Scalars["gate.max_error_recurrence_rate"]; ok {
		if parsed, err := strconv.ParseFloat(scalar.Value, 64); err == nil && parsed >= 0 && parsed <= 1 {
			threshold = parsed
			ref = configRef(scalar)
		}
	}
	status := "pass"
	reason := "Headline ERR is within the configured recurrence gate."
	if headlineERR > threshold {
		status = "fail"
		reason = "Headline ERR exceeds the configured recurrence gate."
	}
	return backtestGateResult{
		Enabled:   true,
		Status:    status,
		Threshold: &threshold,
		Reason:    reason,
		Ref:       ref,
	}
}

func backtestGateThresholdValue(gate backtestGateResult) float64 {
	if gate.Threshold == nil {
		return 0
	}
	return *gate.Threshold
}

func writeBacktestReports(root string, report recurrenceReport, reportDir string) (string, string, *CommandError) {
	cleanReportDir, ok := cleanRepoPath(reportDir)
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
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(jsonRel)), encoded, 0o644); err != nil {
		return "", "", internalError("could not write JSON recurrence report", err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(htmlRel)), []byte(renderBacktestHTML(report)), 0o644); err != nil {
		return "", "", internalError("could not write HTML recurrence report", err)
	}
	return jsonRel, htmlRel, nil
}

func renderBacktestHTML(report recurrenceReport) string {
	var builder strings.Builder
	builder.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>Relia Backtest ")
	builder.WriteString(html.EscapeString(report.ReportID))
	builder.WriteString("</title></head><body>\n")
	builder.WriteString("<h1>Relia Backtest</h1>\n")
	builder.WriteString(fmt.Sprintf("<p>Headline ERR: <strong>%s</strong> (%d confirmed / %d agent failures)</p>\n",
		html.EscapeString(report.Summary.HeadlineERRPercent),
		report.Summary.ConfirmedRecurrenceCount,
		report.Summary.AgentFailureDenominator))
	builder.WriteString("<h2>Confirmed Recurrences</h2>\n<ul>\n")
	for _, pair := range report.ConfirmedRecurrences {
		builder.WriteString("<li>")
		builder.WriteString(html.EscapeString(fmt.Sprintf("PR #%d repeats PR #%d (%s)", pair.CurrentPR, pair.PriorPR, pair.SignatureID)))
		if pair.CurrentURL != "" {
			builder.WriteString(" <a href=\"")
			builder.WriteString(html.EscapeString(pair.CurrentURL))
			builder.WriteString("\">current</a>")
		}
		if pair.PriorURL != "" {
			builder.WriteString(" <a href=\"")
			builder.WriteString(html.EscapeString(pair.PriorURL))
			builder.WriteString("\">prior</a>")
		}
		builder.WriteString("</li>\n")
	}
	builder.WriteString("</ul>\n<h2>Possible Recurrences</h2>\n<ul>\n")
	for _, pair := range report.PossibleRecurrences {
		builder.WriteString("<li>")
		builder.WriteString(html.EscapeString(fmt.Sprintf("PR #%d possibly repeats PR #%d (%s); excluded from headline ERR", pair.CurrentPR, pair.PriorPR, pair.SignatureID)))
		builder.WriteString("</li>\n")
	}
	builder.WriteString("</ul>\n<h2>Flake Discounts</h2>\n<ul>\n")
	for _, flake := range report.FlakeDiscounts {
		builder.WriteString("<li>")
		builder.WriteString(html.EscapeString(fmt.Sprintf("PR #%d %s: %s", flake.PR, flake.SignatureID, flake.Reason)))
		builder.WriteString("</li>\n")
	}
	builder.WriteString("</ul>\n</body></html>\n")
	return builder.String()
}

func saveBacktestBaseline(root string, report recurrenceReport, baselinePath string) *CommandError {
	clean, ok := cleanRepoPath(baselinePath)
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
	clean, ok := cleanRepoPath(baselinePath)
	if !ok {
		return baselineComparison{}, usageError("backtest baseline path must be repo-relative")
	}
	return baselineComparison{
		Status:      "saved",
		Path:        filepath.ToSlash(clean),
		HeadlineERR: roundFloat(headlineERR, 4),
		Delta:       0,
		Stale:       false,
		Reason:      "Saved current headline ERR as the comparison baseline.",
	}, nil
}

func assessResult(args []string, start time.Time) CommandResult {
	options, commandErr := parseAssessArgs(args)
	withFormat := func(result CommandResult) CommandResult {
		if options.Format == "json" {
			result.MachineReadable = true
		}
		return result
	}
	if commandErr != nil {
		return withFormat(errorResult("assess", "assess", commandErr, start))
	}
	wd, err := os.Getwd()
	if err != nil {
		return withFormat(errorResult("assess", "assess", internalError("could not inspect working directory", err), start))
	}
	root, ok := findRepoRoot(wd)
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
	rules, commandErr := loadAssessmentRules(root)
	if commandErr != nil {
		return withFormat(errorResult("assess", "assess", commandErr, start))
	}
	assessment, commandErr := buildRiskAssessment(root, displayPath(root, inputPath), inputContent, touchedPaths, rules)
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

func parseAssessArgs(args []string) (assessOptions, *CommandError) {
	options := assessOptions{Format: "json"}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--input", "--diff", "-i":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("assess requires a path after " + arg)
			}
			options.InputPath = args[index+1]
			index++
		case "--format":
			options.FormatExplicit = true
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("assess requires a value after --format")
			}
			options.Format = args[index+1]
			index++
		default:
			return options, usageError(fmt.Sprintf("unknown assess argument %q", arg))
		}
	}
	if strings.TrimSpace(options.InputPath) == "" {
		return options, usageError("assess requires --input <diff> in offline mode")
	}
	if options.Format != "json" {
		return options, usageError("assess only supports --format json in this task slice")
	}
	return options, nil
}

func parseUnifiedDiffTouchedPaths(content []byte, ref string) ([]string, *CommandError) {
	touched := map[string]bool{}
	inFileHeader := false
	gitFileHeader := false
	currentHeaderAdded := map[string]bool{}
	currentMetadataPaths := []string{}
	hunkOldRemaining := 0
	hunkNewRemaining := 0
	lines := strings.Split(string(content), "\n")
	for index, line := range lines {
		line = strings.TrimRight(line, "\r")
		if hunkOldRemaining > 0 || hunkNewRemaining > 0 {
			hunkOldRemaining, hunkNewRemaining = consumeUnifiedHunkLine(line, hunkOldRemaining, hunkNewRemaining)
			continue
		}
		switch {
		case strings.HasPrefix(line, "diff --git "):
			reconcileSyntheticHeaderPaths(touched, currentHeaderAdded, currentMetadataPaths)
			inFileHeader = true
			currentHeaderAdded = map[string]bool{}
			currentMetadataPaths = nil
			headerPaths, stripGitHeaderPrefix := diffGitHeaderPaths(line)
			gitFileHeader = stripGitHeaderPrefix
			for _, path := range headerPaths {
				if cleanPath, ok := normalizedDiffPath(path, stripGitHeaderPrefix); ok {
					_, existed := touched[cleanPath]
					touched[cleanPath] = true
					if _, tracked := currentHeaderAdded[cleanPath]; !tracked {
						currentHeaderAdded[cleanPath] = !existed
					}
				}
			}
		case inFileHeader && strings.HasPrefix(line, "--- "):
			if !gitFileHeader {
				gitFileHeader = gitStyleUnifiedFileHeader(lines, index)
			}
			addDiffPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "--- ")), gitFileHeader)
		case strings.HasPrefix(line, "--- ") && plainUnifiedFileHeader(lines, index):
			inFileHeader = true
			gitFileHeader = false
			addDiffPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "--- ")), false)
		case inFileHeader && strings.HasPrefix(line, "+++ "):
			addDiffPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "+++ ")), gitFileHeader)
		case inFileHeader && strings.HasPrefix(line, "rename from "):
			currentMetadataPaths = append(currentMetadataPaths, addDiffMetadataPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "rename from "))))
		case inFileHeader && strings.HasPrefix(line, "rename to "):
			currentMetadataPaths = append(currentMetadataPaths, addDiffMetadataPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "rename to "))))
		case inFileHeader && strings.HasPrefix(line, "copy from "):
			currentMetadataPaths = append(currentMetadataPaths, addDiffMetadataPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "copy from "))))
		case inFileHeader && strings.HasPrefix(line, "copy to "):
			currentMetadataPaths = append(currentMetadataPaths, addDiffMetadataPath(touched, strings.TrimSpace(strings.TrimPrefix(line, "copy to "))))
		case strings.HasPrefix(line, "@@"):
			reconcileSyntheticHeaderPaths(touched, currentHeaderAdded, currentMetadataPaths)
			inFileHeader = false
			gitFileHeader = false
			currentMetadataPaths = nil
			hunkOldRemaining, hunkNewRemaining = parseUnifiedHunkCounts(line)
		}
	}
	reconcileSyntheticHeaderPaths(touched, currentHeaderAdded, currentMetadataPaths)
	paths := make([]string, 0, len(touched))
	for touchedPath := range touched {
		paths = append(paths, touchedPath)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, artifactContractError("assess input diff contains no repo-relative paths", ref)
	}
	return paths, nil
}

func plainUnifiedFileHeader(lines []string, index int) bool {
	if index+2 >= len(lines) {
		return false
	}
	return strings.HasPrefix(strings.TrimRight(lines[index+1], "\r"), "+++ ") &&
		strings.HasPrefix(strings.TrimRight(lines[index+2], "\r"), "@@")
}

func gitStyleUnifiedFileHeader(lines []string, index int) bool {
	if index+1 >= len(lines) {
		return false
	}
	oldPath := strings.TrimSpace(strings.TrimPrefix(strings.TrimRight(lines[index], "\r"), "--- "))
	newLine := strings.TrimRight(lines[index+1], "\r")
	if !strings.HasPrefix(newLine, "+++ ") {
		return false
	}
	newPath := strings.TrimSpace(strings.TrimPrefix(newLine, "+++ "))
	return strings.HasPrefix(oldPath, "a/") && strings.HasPrefix(newPath, "b/")
}

func parseUnifiedHunkCounts(line string) (int, int) {
	matches := unifiedHunkHeaderPattern.FindStringSubmatch(line)
	if len(matches) == 0 {
		return 0, 0
	}
	oldCount := 1
	newCount := 1
	if matches[1] != "" {
		if parsed, err := strconv.Atoi(matches[1]); err == nil {
			oldCount = parsed
		}
	}
	if matches[2] != "" {
		if parsed, err := strconv.Atoi(matches[2]); err == nil {
			newCount = parsed
		}
	}
	return oldCount, newCount
}

func consumeUnifiedHunkLine(line string, oldRemaining int, newRemaining int) (int, int) {
	if line == "" {
		return oldRemaining, newRemaining
	}
	switch line[0] {
	case ' ':
		oldRemaining--
		newRemaining--
	case '-':
		oldRemaining--
	case '+':
		newRemaining--
	}
	if oldRemaining < 0 {
		oldRemaining = 0
	}
	if newRemaining < 0 {
		newRemaining = 0
	}
	return oldRemaining, newRemaining
}

func diffGitHeaderPaths(line string) ([]string, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "diff --git "))
	if strings.HasPrefix(rest, "a/") {
		if paths := prefixedGitDiffHeaderPaths(rest); len(paths) > 0 {
			return paths, true
		}
		if paths := identicalNoPrefixDiffHeaderPaths(rest); len(paths) > 0 {
			return paths, false
		}
		return nil, false
	}
	if !strings.HasPrefix(rest, "\"") {
		return identicalNoPrefixDiffHeaderPaths(rest), false
	}
	var paths []string
	for len(rest) > 0 && len(paths) < 2 {
		var path string
		var ok bool
		path, rest, ok = nextDiffHeaderPath(rest)
		if !ok {
			break
		}
		paths = append(paths, path)
	}
	if len(paths) == 2 && quotedOrRawPathHasPrefix(paths[0], "a/") && quotedOrRawPathHasPrefix(paths[1], "b/") {
		return paths, true
	}
	return paths, false
}

func prefixedGitDiffHeaderPaths(rest string) []string {
	type candidate struct {
		left  string
		right string
	}
	var candidates []candidate
	searchStart := 0
	for {
		index := strings.Index(rest[searchStart:], " b/")
		if index < 0 {
			break
		}
		split := searchStart + index
		left := rest[:split]
		right := rest[split+1:]
		if strings.HasPrefix(left, "a/") && strings.HasPrefix(right, "b/") {
			if strings.TrimPrefix(left, "a/") == strings.TrimPrefix(right, "b/") {
				return []string{left, right}
			}
			candidates = append(candidates, candidate{left: left, right: right})
		}
		searchStart = split + len(" b/")
		if searchStart >= len(rest) {
			break
		}
	}
	if len(candidates) == 1 {
		return []string{candidates[0].left, candidates[0].right}
	}
	return nil
}

func identicalNoPrefixDiffHeaderPaths(rest string) []string {
	fields := strings.Fields(rest)
	for split := 1; split < len(fields); split++ {
		left := strings.Join(fields[:split], " ")
		right := strings.Join(fields[split:], " ")
		if left != "" && left == right {
			return []string{left, right}
		}
	}
	return nil
}

func quotedOrRawPathHasPrefix(path string, prefix string) bool {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "\"") {
		if unquoted, err := strconv.Unquote(path); err == nil {
			path = unquoted
		}
	}
	return strings.HasPrefix(path, prefix)
}

func nextDiffHeaderPath(input string) (string, string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", false
	}
	if strings.HasPrefix(input, "\"") {
		escaped := false
		for index := 1; index < len(input); index++ {
			char := input[index]
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				token := input[:index+1]
				if _, err := strconv.Unquote(token); err == nil {
					return token, input[index+1:], true
				}
				return strings.Trim(token, "\""), input[index+1:], true
			}
		}
	}
	if index := strings.IndexAny(input, "\t "); index >= 0 {
		return input[:index], input[index+1:], true
	}
	return input, "", true
}

func addDiffPath(touched map[string]bool, raw string, stripGitPrefix bool) {
	if cleanPath, ok := normalizedDiffPath(raw, stripGitPrefix); ok {
		touched[cleanPath] = true
	}
}

func normalizedDiffPath(raw string, stripGitPrefix bool) (string, bool) {
	pathPart := strings.TrimSpace(raw)
	quoted := strings.HasPrefix(pathPart, "\"")
	if quoted {
		if unquoted, err := strconv.Unquote(pathPart); err == nil {
			pathPart = unquoted
		}
	}
	if !quoted {
		if index := strings.Index(pathPart, "\t"); index >= 0 {
			pathPart = pathPart[:index]
		}
	}
	if pathPart == "" || pathPart == "/dev/null" {
		return "", false
	}
	if stripGitPrefix && (strings.HasPrefix(pathPart, "a/") || strings.HasPrefix(pathPart, "b/")) {
		pathPart = pathPart[2:]
	}
	if clean, ok := cleanRepoPath(pathPart); ok {
		return filepath.ToSlash(clean), true
	}
	return "", false
}

func addDiffMetadataPath(touched map[string]bool, raw string) string {
	addDiffPath(touched, raw, false)
	return raw
}

func reconcileSyntheticHeaderPaths(touched map[string]bool, currentHeaderAdded map[string]bool, metadataPaths []string) {
	for leftIndex, left := range metadataPaths {
		for _, right := range metadataPaths[leftIndex+1:] {
			if stripped, ok := matchingSyntheticMetadataPath(left, right); ok && currentHeaderAdded[stripped] {
				delete(touched, stripped)
			}
		}
	}
}

func matchingSyntheticMetadataPath(left string, right string) (string, bool) {
	leftPath, leftOK := normalizedDiffPath(left, false)
	rightPath, rightOK := normalizedDiffPath(right, false)
	if !leftOK || !rightOK {
		return "", false
	}
	if strings.HasPrefix(leftPath, "a/") && strings.HasPrefix(rightPath, "b/") && strings.TrimPrefix(leftPath, "a/") == strings.TrimPrefix(rightPath, "b/") {
		return strings.TrimPrefix(leftPath, "a/"), true
	}
	if strings.HasPrefix(leftPath, "b/") && strings.HasPrefix(rightPath, "a/") && strings.TrimPrefix(leftPath, "b/") == strings.TrimPrefix(rightPath, "a/") {
		return strings.TrimPrefix(leftPath, "b/"), true
	}
	return "", false
}

func loadAssessmentRules(root string) ([]assessmentRule, *CommandError) {
	patterns := []string{
		filepath.Join(root, "memory", "rules", "*.yaml"),
		filepath.Join(root, "memory", "rules", "*.yml"),
	}
	var paths []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, internalError("could not inspect memory rule artifacts", err)
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)

	var rules []assessmentRule
	for _, rulePath := range paths {
		rule, active, commandErr := readAssessmentRule(root, rulePath)
		if commandErr != nil {
			return nil, commandErr
		}
		if active {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

func readAssessmentRule(root string, rulePath string) (assessmentRule, bool, *CommandError) {
	content, err := os.ReadFile(rulePath)
	if err != nil {
		return assessmentRule{}, false, internalError("could not read memory rule artifact", err)
	}
	rel, err := filepath.Rel(root, rulePath)
	if err != nil {
		rel = rulePath
	}
	rel = filepath.ToSlash(rel)
	document, parseErr := parseYAMLDocument(string(content))
	if parseErr != nil {
		return assessmentRule{}, false, artifactContractError(parseErr.Error(), rel)
	}
	if document.Scalars["status"].Value != "active" {
		return assessmentRule{}, false, nil
	}
	if commandErr := validateActiveAssessmentRuleIdentity(root, document, rel); commandErr != nil {
		return assessmentRule{}, false, commandErr
	}
	confidence, err := strconv.ParseFloat(document.Scalars["confidence"].Value, 64)
	if err != nil {
		return assessmentRule{}, false, artifactContractError("memory rule confidence must be numeric", rel)
	}
	return assessmentRule{
		ID:         document.Scalars["id"].Value,
		Kind:       document.Scalars["kind"].Value,
		Path:       rel,
		Confidence: confidence,
		ScopePaths: yamlListValues(document, "scope.paths"),
		Citations:  assessmentRuleCitations(document),
	}, true, nil
}

func validateActiveAssessmentRuleIdentity(root string, document yamlDocument, rel string) *CommandError {
	required := []string{"object_type", "schema_version", "id", "kind", "status", "statement", "scope", "confidence", "evidence", "provenance", "review", "metadata"}
	for _, key := range required {
		if !hasYAMLPath(document, key) {
			return artifactContractError("memory rule missing required key "+key, rel)
		}
	}
	if document.Scalars["object_type"].Value != "relia.memory_rule" {
		return artifactContractError("memory rule object_type must be relia.memory_rule", configRefWithPath(rel, document.Scalars["object_type"]))
	}
	if document.Scalars["schema_version"].Value != commandSchemaVersion {
		return artifactContractError("memory rule schema_version must be "+commandSchemaVersion, rel)
	}
	kind := document.Scalars["kind"].Value
	if kind != "avoid" && kind != "playbook" {
		return artifactContractError("memory rule kind must be avoid or playbook", configRefWithPath(rel, document.Scalars["kind"]))
	}
	if kind == "playbook" && !assessmentRuleHasPositivePlaybookEvidence(document) {
		return artifactContractError("playbook memory rule must cite at least one fix_held or merged_clean provenance outcome", rel)
	}
	if len(document.Lists["scope.paths"]) == 0 && len(document.Lists["scope.signals"]) == 0 {
		return artifactContractError("memory rule must declare at least one scope path or signal", rel)
	}
	for _, scopePath := range document.Lists["scope.paths"] {
		if !repoPathExists(root, scopePath.Value) {
			return artifactContractError("memory rule scope path does not exist in the repo", configRefWithPath(rel, scopePath))
		}
	}
	reviewLabel, ok := document.Scalars["review.label"]
	if !ok {
		return artifactContractError("memory rule missing required key review.label", rel)
	}
	if reviewLabel.Value != "accepted" {
		return artifactContractError("active memory rule review.label must be accepted", configRefWithPath(rel, reviewLabel))
	}
	statementOrigin, ok := document.Scalars["review.statement_origin"]
	if !ok {
		return artifactContractError("memory rule missing required key review.statement_origin", rel)
	}
	switch statementOrigin.Value {
	case "llm_drafted", "cluster_summary", "human_authored":
	default:
		return artifactContractError("memory rule review.statement_origin is invalid", configRefWithPath(rel, statementOrigin))
	}
	if len(document.Lists["evidence.experiences"]) == 0 {
		return artifactContractError("memory rule must cite at least one experience", rel)
	}
	evidenceCount, ok := document.Scalars["evidence.count"]
	if !ok {
		return artifactContractError("memory rule missing required key evidence.count", rel)
	}
	count, err := strconv.Atoi(evidenceCount.Value)
	if err != nil || count < 1 {
		return artifactContractError("memory rule evidence.count must be at least 1", configRefWithPath(rel, evidenceCount))
	}
	contradictionsScalar, ok := document.Scalars["evidence.contradictions"]
	if !ok {
		return artifactContractError("memory rule missing required key evidence.contradictions", rel)
	}
	contradictions, err := strconv.Atoi(contradictionsScalar.Value)
	if err != nil || contradictions < 0 {
		return artifactContractError("memory rule evidence.contradictions must be at least 0", configRefWithPath(rel, contradictionsScalar))
	}
	provenanceEntries := document.Lists["provenance"]
	if len(provenanceEntries) == 0 {
		return artifactContractError("memory rule must include at least one provenance entry", rel)
	}
	provenanceMaps := document.ListMaps["provenance"]
	if len(provenanceMaps) != len(provenanceEntries) {
		return artifactContractError("memory rule provenance entries must include pr and outcome", rel)
	}
	for _, provenance := range provenanceMaps {
		pr, ok := provenance["pr"]
		if !ok {
			return artifactContractError("memory rule provenance entry missing pr", rel)
		}
		prNumber, err := strconv.Atoi(pr.Value)
		if err != nil || prNumber < 1 {
			return artifactContractError("memory rule provenance pr must be at least 1", configRefWithPath(rel, pr))
		}
		outcome, ok := provenance["outcome"]
		if !ok {
			return artifactContractError("memory rule provenance entry missing outcome", rel)
		}
		switch outcome.Value {
		case "ci_failure", "revert", "review_correction", "fix_held", "merged_clean":
		default:
			return artifactContractError("memory rule provenance outcome is invalid", configRefWithPath(rel, outcome))
		}
	}
	return nil
}

func assessmentRuleHasPositivePlaybookEvidence(document yamlDocument) bool {
	for _, provenance := range document.ListMaps["provenance"] {
		outcome, ok := provenance["outcome"]
		if !ok {
			continue
		}
		if outcome.Value == "fix_held" || outcome.Value == "merged_clean" {
			return true
		}
	}
	return false
}

func assessmentRuleCitations(document yamlDocument) []assessmentRuleCitation {
	var citations []assessmentRuleCitation
	for _, provenance := range document.ListMaps["provenance"] {
		url, ok := provenance["url"]
		if !ok || strings.TrimSpace(url.Value) == "" {
			continue
		}
		prNumber := 0
		if pr, ok := provenance["pr"]; ok {
			prNumber, _ = strconv.Atoi(pr.Value)
		}
		outcome := ""
		if value, ok := provenance["outcome"]; ok {
			outcome = value.Value
		}
		citations = append(citations, assessmentRuleCitation{
			URL:     url.Value,
			PR:      prNumber,
			Outcome: outcome,
		})
	}
	return uniqueAssessmentRuleCitations(citations)
}

func uniqueAssessmentRuleCitations(citations []assessmentRuleCitation) []assessmentRuleCitation {
	seen := map[string]bool{}
	var unique []assessmentRuleCitation
	for _, citation := range citations {
		key := fmt.Sprintf("%s\x00%d\x00%s", citation.URL, citation.PR, citation.Outcome)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, citation)
	}
	return unique
}

func buildRiskAssessment(root string, inputRef string, content []byte, touchedPaths []string, rules []assessmentRule) (riskAssessment, *CommandError) {
	matches := []riskAssessmentMatch{}
	citations := []string{}
	highestAvoidConfidence := -1.0
	hasPlaybookCoverage := false
	for _, rule := range rules {
		if !assessmentRuleMatchesTouchedPath(root, rule, touchedPaths) {
			continue
		}
		if strings.TrimSpace(rule.ID) == "" {
			return riskAssessment{}, provenanceIntegrityError("matched active memory rule id must be non-empty for assessment", rule.Path)
		}
		servedCitationRefs := servedAssessmentRuleCitations(rule)
		servedCitations := assessmentRuleCitationURLs(servedCitationRefs)
		if len(servedCitations) == 0 {
			return riskAssessment{}, provenanceIntegrityError("matched active memory rule must include citation URLs for assessment", rule.Path)
		}
		if math.IsNaN(rule.Confidence) || math.IsInf(rule.Confidence, 0) || rule.Confidence < 0 || rule.Confidence > 1 {
			return riskAssessment{}, provenanceIntegrityError("matched active memory rule confidence must be between 0 and 1 for assessment", rule.Path)
		}
		if commandErr := validateServedAssessmentRuleCitations(rule, servedCitationRefs); commandErr != nil {
			return riskAssessment{}, commandErr
		}
		matches = append(matches, riskAssessmentMatch{
			RuleID:     rule.ID,
			Confidence: rule.Confidence,
		})
		citations = append(citations, servedCitations...)
		if rule.Kind == "playbook" {
			hasPlaybookCoverage = true
		} else if rule.Confidence > highestAvoidConfidence {
			highestAvoidConfidence = rule.Confidence
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Confidence == matches[j].Confidence {
			return matches[i].RuleID < matches[j].RuleID
		}
		return matches[i].Confidence > matches[j].Confidence
	})
	citations = uniqueStrings(citations)
	if citations == nil {
		citations = []string{}
	}
	riskLevel := "no_coverage"
	if highestAvoidConfidence >= 0.75 {
		riskLevel = "match_high"
	} else if highestAvoidConfidence >= 0 {
		riskLevel = "match_medium"
	} else if hasPlaybookCoverage {
		riskLevel = "covered_clean"
	}
	return riskAssessment{
		ObjectType:    "relia.risk_assessment",
		SchemaVersion: commandSchemaVersion,
		AssessmentID:  "assess_" + shortHash(inputRef+"|"+sha256String(string(content))+"|"+strings.Join(touchedPaths, "\x00")),
		RiskLevel:     riskLevel,
		Matches:       matches,
		Citations:     citations,
		Metadata: map[string]any{
			"input_path":               inputRef,
			"diff_fingerprint":         sha256String(string(content)),
			"touched_paths":            touchedPaths,
			"repo_relative_paths_only": true,
			"redaction_status":         "customer_safe",
		},
	}, nil
}

func validateServedAssessmentRuleCitations(rule assessmentRule, servedCitationRefs []assessmentRuleCitation) *CommandError {
	for _, citation := range servedCitationRefs {
		prNumber, ok := gitHubPullRequestURLNumber(citation.URL)
		if !ok {
			return provenanceIntegrityError("matched active memory rule citation URL must be an https://github.com/<owner>/<repo>/pull/<number> URL", rule.Path)
		}
		if citation.PR <= 0 || prNumber != citation.PR {
			return provenanceIntegrityError("matched active memory rule citation URL pull number must match provenance pr", rule.Path)
		}
	}
	return nil
}

func servedAssessmentRuleCitationURLs(rule assessmentRule) []string {
	return assessmentRuleCitationURLs(servedAssessmentRuleCitations(rule))
}

func servedAssessmentRuleCitations(rule assessmentRule) []assessmentRuleCitation {
	var refs []assessmentRuleCitation
	for _, citation := range rule.Citations {
		if rule.Kind == "playbook" && citation.Outcome != "fix_held" && citation.Outcome != "merged_clean" {
			continue
		}
		refs = append(refs, citation)
	}
	return uniqueAssessmentRuleCitations(refs)
}

func assessmentRuleCitationURLs(refs []assessmentRuleCitation) []string {
	var citations []string
	for _, citation := range refs {
		citations = append(citations, citation.URL)
	}
	return uniqueStrings(citations)
}

func assessmentRuleMatchesTouchedPath(root string, rule assessmentRule, touchedPaths []string) bool {
	for _, rawScopePath := range rule.ScopePaths {
		scopePath, directoryScope, ok := normalizeAssessmentScopePath(root, rawScopePath)
		if !ok {
			continue
		}
		for _, touchedPath := range touchedPaths {
			touchedPath = filepath.ToSlash(filepath.Clean(filepath.FromSlash(touchedPath)))
			if scopePatternMatches(scopePath, touchedPath) || scopePath == touchedPath || directoryScopeMatches(scopePath, touchedPath, directoryScope) {
				return true
			}
		}
	}
	return false
}

func normalizeAssessmentScopePath(root string, raw string) (string, bool, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false, false
	}
	slashPath := filepath.ToSlash(trimmed)
	directoryScope := strings.HasSuffix(slashPath, "/") && !hasGlobMagic(slashPath)
	clean, ok := cleanRepoPath(slashPath)
	if !ok {
		return "", false, false
	}
	scopePath := filepath.ToSlash(clean)
	if !directoryScope && !hasGlobMagic(scopePath) {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(scopePath)))
		if err == nil {
			directoryScope = info.IsDir()
		} else if historicalDirectoryScope(root, scopePath) {
			directoryScope = true
		}
	}
	return scopePath, directoryScope, true
}

func directoryScopeMatches(scopePath string, touchedPath string, directoryScope bool) bool {
	if !directoryScope {
		return false
	}
	return touchedPath == scopePath || strings.HasPrefix(touchedPath, scopePath+"/")
}

func historicalDirectoryScope(root string, scopePath string) bool {
	output, err := exec.Command("git", "-C", root, "log", "--all", "--name-only", "--format=", "--", scopePath).Output()
	if err != nil {
		return false
	}
	prefix := strings.TrimSuffix(scopePath, "/") + "/"
	for _, line := range strings.Split(string(output), "\n") {
		clean, ok := cleanRepoPath(line)
		if ok && strings.HasPrefix(filepath.ToSlash(clean), prefix) {
			return true
		}
	}
	return false
}

func readReliaConfig(root string) (yamlDocument, *CommandError) {
	content, err := os.ReadFile(filepath.Join(root, defaultConfigFile))
	if err != nil {
		return yamlDocument{}, internalError("could not read relia.yaml", err)
	}
	document, parseErr := parseYAMLDocument(string(content))
	if parseErr != nil {
		return yamlDocument{}, configError(parseErr.Error())
	}
	return document, nil
}

func resolveInputPath(root string, input string) string {
	if filepath.IsAbs(input) {
		return filepath.Clean(input)
	}
	return filepath.Clean(filepath.Join(root, input))
}

func parseIngestEvents(content []byte, ref string) ([]map[string]any, *CommandError) {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil, artifactContractError("ingest input is empty", ref)
	}
	var decoded any
	if err := decodeJSONUseNumber(trimmed, &decoded); err == nil {
		return ingestEventsFromJSON(decoded, ref)
	}
	var events []map[string]any
	for lineNumber, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event map[string]any
		if err := decodeJSONUseNumber(line, &event); err != nil {
			return nil, artifactContractError(fmt.Sprintf("ingest JSONL line %d is not a JSON object", lineNumber+1), ref)
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, artifactContractError("ingest input contains no events", ref)
	}
	return events, nil
}

func decodeJSONUseNumber(input string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func ingestEventsFromJSON(value any, ref string) ([]map[string]any, *CommandError) {
	switch typed := value.(type) {
	case []any:
		return ingestEventsFromArray(typed, ref)
	case map[string]any:
		if nested, ok := typed["events"]; ok {
			events, ok := nested.([]any)
			if !ok {
				return nil, artifactContractError("ingest events must be an array", ref)
			}
			return ingestEventsFromArray(events, ref)
		}
		return []map[string]any{typed}, nil
	default:
		return nil, artifactContractError("ingest input must be a JSON object, array, or JSONL stream", ref)
	}
}

func ingestEventsFromArray(values []any, ref string) ([]map[string]any, *CommandError) {
	if len(values) == 0 {
		return nil, artifactContractError("ingest input contains no events", ref)
	}
	events := make([]map[string]any, 0, len(values))
	for index, value := range values {
		event, ok := value.(map[string]any)
		if !ok {
			return nil, artifactContractError(fmt.Sprintf("ingest event %d must be a JSON object", index+1), ref)
		}
		events = append(events, event)
	}
	return events, nil
}

func normalizeExperienceRecord(config yamlDocument, event map[string]any, index int, ref string) (experienceRecord, bool, *CommandError) {
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
		if !strings.HasPrefix(value, "https://github.com/") {
			return experienceProvenance{}, provenanceIntegrityError("experience provenance URL must be an https://github.com/ URL", ref)
		}
	}
	return experienceProvenance{URLs: urls}, nil
}

func persistExperienceRecords(root string, records []experienceRecord) ([]string, *CommandError) {
	if len(records) == 0 {
		return []string{}, nil
	}
	grouped := map[string][]experienceRecord{}
	for _, record := range records {
		recordedAt, err := time.Parse(time.RFC3339, record.RecordedAt)
		if err != nil {
			return nil, artifactContractError("experience recorded_at must remain RFC3339 before persistence", record.ExperienceID)
		}
		shard := filepath.ToSlash(filepath.Join(".relia", "experiences", recordedAt.UTC().Format("2006-01")+".jsonl"))
		grouped[shard] = append(grouped[shard], record)
	}
	shards := make([]string, 0, len(grouped))
	for shard := range grouped {
		shards = append(shards, shard)
	}
	sort.Strings(shards)
	plans := make([]experienceShardWritePlan, 0, len(shards))
	for _, shard := range shards {
		plan, commandErr := prepareExperienceShardWrite(filepath.Join(root, filepath.FromSlash(shard)), grouped[shard])
		if commandErr != nil {
			return nil, commandErr
		}
		plans = append(plans, plan)
	}
	for _, plan := range plans {
		if commandErr := writeExperienceShard(plan); commandErr != nil {
			return nil, commandErr
		}
	}
	return shards, nil
}

type experienceShardWritePlan struct {
	Path    string
	Content []byte
}

func prepareExperienceShardWrite(path string, records []experienceRecord) (experienceShardWritePlan, *CommandError) {
	order := []string{}
	byID := map[string]json.RawMessage{}
	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return experienceShardWritePlan{}, internalError("could not read existing experience shard", err)
	}
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var existing map[string]any
		if err := json.Unmarshal([]byte(line), &existing); err != nil {
			return experienceShardWritePlan{}, provenanceIntegrityError(fmt.Sprintf("existing experience shard line %d is not valid JSON", lineNumber+1), filepath.ToSlash(path))
		}
		experienceID := stringFromAny(existing["experience_id"])
		if experienceID == "" {
			return experienceShardWritePlan{}, provenanceIntegrityError(fmt.Sprintf("existing experience shard line %d missing experience_id", lineNumber+1), filepath.ToSlash(path))
		}
		if _, ok := byID[experienceID]; !ok {
			order = append(order, experienceID)
		}
		byID[experienceID] = append(json.RawMessage(nil), []byte(line)...)
	}
	for _, record := range records {
		content, err := json.Marshal(record)
		if err != nil {
			return experienceShardWritePlan{}, internalError("could not encode experience record", err)
		}
		if _, ok := byID[record.ExperienceID]; !ok {
			order = append(order, record.ExperienceID)
		}
		byID[record.ExperienceID] = content
	}
	var builder strings.Builder
	for _, experienceID := range order {
		builder.Write(byID[experienceID])
		builder.WriteByte('\n')
	}
	return experienceShardWritePlan{Path: path, Content: []byte(builder.String())}, nil
}

func writeExperienceShard(plan experienceShardWritePlan) *CommandError {
	if err := os.MkdirAll(filepath.Dir(plan.Path), 0o755); err != nil {
		return internalError("could not create experience shard directory", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(plan.Path), "."+filepath.Base(plan.Path)+".tmp-*")
	if err != nil {
		return internalError("could not create temporary experience shard", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := tempFile.Write(plan.Content); err != nil {
		_ = tempFile.Close()
		return internalError("could not write temporary experience shard", err)
	}
	if err := tempFile.Close(); err != nil {
		return internalError("could not close temporary experience shard", err)
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return internalError("could not set temporary experience shard permissions", err)
	}
	if err := os.Rename(tempPath, plan.Path); err != nil {
		return internalError("could not write experience shard", err)
	}
	cleanup = false
	return nil
}

func redactForPersistence(event map[string]any, ref string) (any, *CommandError) {
	return redactValue(event, nil, ref)
}

func redactValue(value any, fieldPath []string, ref string) (any, *CommandError) {
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, child := range typed {
			childPath := append(append([]string{}, fieldPath...), key)
			if commandErr := validateRedactedMapKey(key, fieldPath, ref); commandErr != nil {
				return nil, commandErr
			}
			if isSecretField(key) {
				redacted[key] = "[REDACTED:secret]"
				continue
			}
			next, commandErr := redactValue(child, childPath, ref)
			if commandErr != nil {
				return nil, commandErr
			}
			redacted[key] = next
		}
		return redacted, nil
	case []any:
		redacted := make([]any, 0, len(typed))
		for index, child := range typed {
			childPath := append(append([]string{}, fieldPath...), strconv.Itoa(index))
			next, commandErr := redactValue(child, childPath, ref)
			if commandErr != nil {
				return nil, commandErr
			}
			redacted = append(redacted, next)
		}
		return redacted, nil
	case string:
		return redactStringValue(typed, fieldPath, ref)
	default:
		return value, nil
	}
}

func validateRedactedMapKey(key string, fieldPath []string, ref string) *CommandError {
	keyPath := append(append([]string{}, fieldPath...), "<key>")
	redacted, commandErr := redactStringValue(key, keyPath, ref)
	if commandErr != nil {
		return commandErr
	}
	if redacted != key {
		pathRef := strings.Join(fieldPath, ".")
		if pathRef == "" {
			pathRef = "<root>"
		}
		return redactionSafetyError(fmt.Sprintf("secret-shaped object key at %s", pathRef), ref)
	}
	return nil
}

func redactStringValue(value string, fieldPath []string, ref string) (string, *CommandError) {
	redacted := value
	for _, pattern := range knownSecretPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED:token]")
	}
	if token := unsafeEntropyToken(redacted, fieldPath); token != "" {
		pathRef := strings.Join(fieldPath, ".")
		if pathRef == "" {
			pathRef = "<root>"
		}
		return "", redactionSafetyError(fmt.Sprintf("unrecognized high-entropy value at %s", pathRef), ref)
	}
	return redacted, nil
}

func isSecretField(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	switch normalized {
	case "token", "tokens", "secret", "secrets", "password", "passwords", "credential", "credentials", "api_key", "api_keys", "access_token", "access_tokens", "refresh_token", "refresh_tokens", "private_key", "private_keys", "client_secret", "client_secrets":
		return true
	}
	return strings.HasSuffix(normalized, "_token") ||
		strings.HasSuffix(normalized, "_tokens") ||
		strings.HasSuffix(normalized, "_secret") ||
		strings.HasSuffix(normalized, "_secrets") ||
		strings.HasSuffix(normalized, "_password") ||
		strings.HasSuffix(normalized, "_passwords") ||
		strings.Contains(normalized, "credential")
}

func unsafeEntropyToken(value string, fieldPath []string) string {
	if isGitHubProvenanceURLField(fieldPath) {
		if token := unsafeGitHubURLPathEntropyToken(value); token != "" {
			return token
		}
		if validGitHubProvenanceURLShape(value) {
			return ""
		}
	}
	if entropySafeFieldValue(fieldPath, value) {
		return ""
	}
	return unsafeEntropyTokenInString(value)
}

func unsafeEntropyTokenInString(value string) string {
	return unsafeEntropyTokenInStringWithSlashPolicy(value, true)
}

func unsafeEntropyTokenInPath(value string) string {
	return unsafeEntropyTokenInStringWithSlashPolicy(value, false)
}

func unsafeEntropyTokenInStringWithSlashPolicy(value string, allowSlash bool) string {
	candidates := strings.FieldsFunc(value, func(r rune) bool {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '+' || r == '_' || r == '-' || r == '=' {
			return false
		}
		if allowSlash && r == '/' {
			return false
		}
		return true
	})
	for _, candidate := range candidates {
		candidate = strings.Trim(candidate, "-_=+/")
		if len(candidate) < 32 {
			continue
		}
		if !hasMixedSecretAlphabet(candidate) {
			continue
		}
		if shannonEntropy(candidate) > 4.2 {
			return candidate
		}
	}
	return ""
}

func entropySafeFieldValue(fieldPath []string, value string) bool {
	for _, part := range fieldPath {
		normalized := strings.ToLower(part)
		switch normalized {
		case "commit", "commits":
			return validGitCommitHash(value)
		case "signature_id":
			return validSignatureIDValue(value)
		case "diff_fingerprint", "message_fingerprint", "digest", "checksum":
			return validHashLikeValue(value)
		}
		if strings.Contains(normalized, "fingerprint") {
			return validHashLikeValue(value)
		}
	}
	return false
}

func isGitHubProvenanceURLField(fieldPath []string) bool {
	for index, part := range fieldPath {
		normalized := strings.ToLower(part)
		switch normalized {
		case "pr_url", "check_run_url", "revert_url", "review_url", "provenance_urls":
			return true
		case "urls":
			if index > 0 && strings.ToLower(fieldPath[index-1]) == "provenance" {
				return true
			}
		}
	}
	return false
}

func validGitHubProvenanceURLShape(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" &&
		strings.EqualFold(parsed.Host, "github.com") &&
		parsed.User == nil &&
		parsed.RawQuery == "" &&
		parsed.Fragment == "" &&
		strings.Trim(parsed.Path, "/") != ""
}

func gitHubPullRequestURLNumber(value string) (int, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	if parsed.Scheme != "https" ||
		!strings.EqualFold(parsed.Host, "github.com") ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return 0, false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pull" {
		return 0, false
	}
	if parts[0] == "" || parts[1] == "" {
		return 0, false
	}
	number, err := strconv.Atoi(parts[3])
	return number, err == nil && number > 0
}

func unsafeGitHubURLPathEntropyToken(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") {
		return ""
	}
	for _, path := range []string{parsed.EscapedPath(), parsed.Path} {
		trimmed := strings.Trim(path, "/")
		if token := unsafeEntropyTokenInPath(trimmed); token != "" {
			return token
		}
		if token := unsafeSlashBearingEntropyTokenInPath(trimmed); token != "" {
			return token
		}
	}
	if unescaped, err := url.PathUnescape(parsed.EscapedPath()); err == nil {
		trimmed := strings.Trim(unescaped, "/")
		if token := unsafeEntropyTokenInPath(trimmed); token != "" {
			return token
		}
		if token := unsafeSlashBearingEntropyTokenInPath(trimmed); token != "" {
			return token
		}
	}
	return ""
}

func unsafeSlashBearingEntropyTokenInPath(path string) string {
	rawSegments := strings.Split(strings.Trim(path, "/"), "/")
	segments := unsafeGitHubPathTokenSegments(path)
	if len(segments) == 0 {
		return ""
	}
	for start := 0; start < len(segments); start++ {
		candidateSegments := make([]string, 0, len(segments)-start)
		for end := start; end < len(segments); end++ {
			segment := strings.Trim(segments[end], "-_=+")
			if segment == "" {
				if githubOwnerRepoRouteBoundary(rawSegments, end) &&
					(!suspiciousGitHubOwnerRepoTokenPrefix(candidateSegments) ||
						githubOwnerRepoRouteBoundaryHasSafeTypedPayload(rawSegments, end) ||
						githubOwnerRepoRouteBoundaryHasSafeUntypedPayload(rawSegments, end)) {
					candidateSegments = candidateSegments[:0]
				}
				continue
			}
			if !entropyPathCandidateFragment(segment) {
				break
			}
			candidateSegments = append(candidateSegments, segment)
			candidate := strings.Join(candidateSegments, "/")
			candidateWithoutSlash := strings.ReplaceAll(candidate, "/", "")
			if len(candidateSegments) < 2 {
				continue
			}
			if len(candidateWithoutSlash) < 32 {
				continue
			}
			if !hasMixedSecretAlphabet(candidateWithoutSlash) {
				continue
			}
			if shannonEntropy(candidate) > 4.2 {
				return candidate
			}
		}
	}
	return ""
}

func suspiciousGitHubOwnerRepoTokenPrefix(segments []string) bool {
	if len(segments) < 2 {
		return false
	}
	candidate := strings.Join(segments, "/")
	candidateWithoutSlash := strings.ReplaceAll(candidate, "/", "")
	if len(candidateWithoutSlash) < 16 {
		return false
	}
	if !hasMixedSecretAlphabet(candidateWithoutSlash) {
		return false
	}
	return shannonEntropy(candidate) > 3.2
}

func githubOwnerRepoRouteBoundary(rawSegments []string, index int) bool {
	if index != 2 || len(rawSegments) <= index {
		return false
	}
	route := strings.ToLower(strings.Trim(rawSegments[index], "-_=+"))
	if !safeGitHubRouteSegment(route) {
		return false
	}
	switch route {
	case "commit", "commits":
		return len(rawSegments) > index+1 && validGitCommitHash(rawSegments[index+1])
	case "pull", "pulls", "issues":
		return len(rawSegments) > index+1 && isDecimalString(rawSegments[index+1])
	case "runs":
		return len(rawSegments) > index+1 && isDecimalString(rawSegments[index+1])
	case "actions":
		return len(rawSegments) > index+1 && strings.EqualFold(rawSegments[index+1], "runs")
	case "checks", "suites", "workflow-runs", "tree", "blob", "compare":
		return len(rawSegments) > index+1
	default:
		return false
	}
}

func githubOwnerRepoRouteBoundaryHasSafeTypedPayload(rawSegments []string, index int) bool {
	if index != 2 || len(rawSegments) <= index {
		return false
	}
	route := strings.ToLower(strings.Trim(rawSegments[index], "-_=+"))
	switch route {
	case "commit", "commits":
		return len(rawSegments) > index+1 && validGitCommitHash(rawSegments[index+1])
	case "pull", "pulls", "issues":
		return len(rawSegments) > index+1 && isDecimalString(rawSegments[index+1])
	case "runs":
		return len(rawSegments) > index+1 && isDecimalString(rawSegments[index+1])
	case "actions":
		return len(rawSegments) > index+2 &&
			strings.EqualFold(strings.Trim(rawSegments[index+1], "-_=+"), "runs") &&
			isDecimalString(strings.Trim(rawSegments[index+2], "-_=+"))
	default:
		return false
	}
}

func githubOwnerRepoRouteBoundaryHasSafeUntypedPayload(rawSegments []string, index int) bool {
	if index != 2 || len(rawSegments) <= index {
		return false
	}
	route := strings.ToLower(strings.Trim(rawSegments[index], "-_=+"))
	switch route {
	case "tree", "blob":
		return githubUntypedRoutePayloadSafe(rawSegments[index+1:])
	default:
		return false
	}
}

func githubUntypedRoutePayloadSafe(rawSegments []string) bool {
	if len(rawSegments) == 0 {
		return false
	}
	for _, rawSegment := range rawSegments {
		segment := strings.Trim(rawSegment, "-_=+")
		if segment == "" {
			return false
		}
		if unsafeEntropyTokenInPath(segment) != "" {
			return false
		}
	}
	return unsafeSlashBearingEntropyTokenInPath(strings.Join(rawSegments, "/")) == ""
}

func unsafeGitHubPathTokenSegments(path string) []string {
	rawSegments := strings.Split(strings.Trim(path, "/"), "/")
	segments := make([]string, 0, len(rawSegments))
	for index, rawSegment := range rawSegments {
		segment := strings.Trim(rawSegment, "-_=+")
		if segment == "" {
			segments = append(segments, "")
			continue
		}
		if structuralGitHubRouteSegment(rawSegments, index) {
			segments = append(segments, "")
			continue
		}
		segments = append(segments, segment)
	}
	return segments
}

func structuralGitHubRouteSegment(rawSegments []string, index int) bool {
	if githubOwnerRepoRouteBoundary(rawSegments, index) {
		return true
	}
	if index == 3 &&
		len(rawSegments) > 3 &&
		strings.EqualFold(strings.Trim(rawSegments[2], "-_=+"), "actions") &&
		strings.EqualFold(strings.Trim(rawSegments[3], "-_=+"), "runs") {
		return true
	}
	return false
}

func safeGitHubRouteSegment(segment string) bool {
	normalized := strings.ToLower(segment)
	switch normalized {
	case "actions", "runs", "pull", "pulls", "issues", "commit", "commits", "tree", "blob", "compare", "checks", "suites", "workflow-runs":
		return true
	}
	return false
}

func isDecimalString(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func entropyPathCandidateFragment(segment string) bool {
	if segment == "" {
		return false
	}
	for _, r := range segment {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '+', r == '_', r == '-', r == '=':
		default:
			return false
		}
	}
	return true
}

func validGitCommitHash(value string) bool {
	return isHexString(strings.TrimSpace(value), 6, 64)
}

func validSignatureIDValue(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "sig_") {
		return isHexString(strings.TrimPrefix(value, "sig_"), 6, 64)
	}
	return validHashLikeValue(value)
}

func validHashLikeValue(value string) bool {
	value = strings.TrimSpace(value)
	if isHexString(value, 6, 128) {
		return true
	}
	for prefix, length := range map[string]int{
		"sha1:":   40,
		"sha256:": 64,
		"sha512:": 128,
	} {
		if strings.HasPrefix(strings.ToLower(value), prefix) {
			return isHexString(value[len(prefix):], length, length)
		}
	}
	return false
}

func isHexString(value string, minLength int, maxLength int) bool {
	if len(value) < minLength || len(value) > maxLength {
		return false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func hasMixedSecretAlphabet(value string) bool {
	hasLower := false
	hasUpper := false
	hasDigit := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	categories := 0
	for _, present := range []bool{hasLower, hasUpper, hasDigit} {
		if present {
			categories++
		}
	}
	return categories >= 2
}

func shannonEntropy(value string) float64 {
	if value == "" {
		return 0
	}
	counts := map[rune]int{}
	for _, r := range value {
		counts[r]++
	}
	length := float64(len([]rune(value)))
	entropy := 0.0
	for _, count := range counts {
		probability := float64(count) / length
		entropy -= probability * math.Log2(probability)
	}
	return entropy
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
	scalars := document.Lists[path]
	values := make([]string, 0, len(scalars))
	for _, scalar := range scalars {
		if strings.TrimSpace(scalar.Value) != "" {
			values = append(values, scalar.Value)
		}
	}
	return values
}

func yamlListValuesWithMapFields(document yamlDocument, path string, fields ...string) []string {
	values := yamlListValues(document, path)
	for _, mapping := range document.ListMaps[path] {
		for _, field := range fields {
			scalar, ok := mapping[field]
			if !ok || strings.TrimSpace(scalar.Value) == "" {
				continue
			}
			values = append(values, scalar.Value)
		}
	}
	return uniqueStrings(values)
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
	if data == nil {
		data = map[string]any{}
	}
	data["message"] = message
	return baseResult(command, mode, "pass", ExitSuccess, start, data)
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
	result := baseResult(command, mode, "error", commandErr.ExitCode, start, nil)
	result.Errors = append(result.Errors, *commandErr)
	if commandErr.ExitCode == ExitRedactionSafety {
		result.RedactionStatus = "failed_closed"
	}
	return result
}

func baseResult(command string, mode string, status string, exitCode int, start time.Time, data map[string]any) CommandResult {
	return CommandResult{
		ObjectType:    commandResultObjectType,
		SchemaVersion: commandSchemaVersion,
		Command:       command,
		Status:        status,
		Mode:          mode,
		ExitCode:      exitCode,
		Warnings:      []Finding{},
		Errors:        []CommandError{},
		Artifacts:     []ArtifactRef{},
		EvidenceRefs: []string{
			"docs/product/prd.md#command-model",
			"docs/dev/dev_guides.md#agent-native-cli-policy",
			"schemas/command-result.schema.json",
		},
		DurationMS:      time.Since(start).Milliseconds(),
		RedactionStatus: "not_applicable",
		Metadata: map[string]any{
			"relia_version":  reliaVersion,
			"schema_ref":     "schemas/command-result.schema.json",
			"schema_version": commandSchemaVersion,
		},
		Data: data,
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

func findRepoRoot(start string) (string, bool) {
	current := filepath.Clean(start)
	for {
		goMod := filepath.Join(current, "go.mod")
		content, err := os.ReadFile(goMod)
		if err == nil && strings.Contains(string(content), "module github.com/Clyra-AI/relia") {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func ensureArtifactSkeleton(root string) error {
	for _, dir := range artifactSkeletonDirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func ensureReliaGitIgnore(root string) error {
	path := filepath.Join(root, ".gitignore")
	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if gitIgnoreContainsRelia(content) {
		return nil
	}
	next := strings.TrimRight(string(content), "\n")
	if next != "" {
		next += "\n"
	}
	next += ".relia/\n"
	return os.WriteFile(path, []byte(next), 0o644)
}

func gitIgnoreContainsRelia(content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		switch strings.TrimSpace(line) {
		case ".relia", ".relia/", ".relia/*":
			return true
		}
	}
	return false
}

func validateReliaConfig(root string) ([]Finding, *CommandError) {
	path := filepath.Join(root, defaultConfigFile)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, internalError("could not read relia.yaml", err)
	}
	document, parseErr := parseYAMLDocument(string(content))
	if parseErr != nil {
		return nil, configError(parseErr.Error())
	}

	requiredExact := map[string]string{
		"version":                         "1",
		"artifacts.schema_version":        commandSchemaVersion,
		"artifacts.relia_version":         reliaVersion,
		"artifacts.root":                  ".relia",
		"artifacts.commit_experiences":    "false",
		"privacy.local_only":              "true",
		"privacy.send_code":               "false",
		"privacy.send_diffs":              "false",
		"privacy.send_logs":               "false",
		"privacy.send_experience_records": "false",
		"privacy.share_scope":             "private",
		"redaction.schema_version":        commandSchemaVersion,
		"redaction.entropy_scan":          "true",
		"redaction.fail_closed":           "true",
		"redaction.standard_token_shapes": "true",
		"models.local_manifest":           ".relia/models/manifest.json",
		"serve.advisory_only":             "true",
	}
	for key, want := range requiredExact {
		scalar, ok := document.Scalars[key]
		if !ok {
			return nil, configError(fmt.Sprintf("relia.yaml missing required key %s", key))
		}
		if scalar.Value != want {
			switch key {
			case "artifacts.commit_experiences", "privacy.local_only", "privacy.send_code", "privacy.send_diffs", "privacy.send_logs", "privacy.send_experience_records", "privacy.share_scope":
				return nil, artifactContractError(fmt.Sprintf("%s must be %s for the MVP artifact contract", key, want), configRef(scalar))
			case "redaction.entropy_scan", "redaction.fail_closed", "redaction.standard_token_shapes":
				return nil, redactionSafetyError(fmt.Sprintf("%s must be %s", key, want), configRef(scalar))
			default:
				return nil, configError(fmt.Sprintf("%s must be %s", key, want))
			}
		}
	}

	var warnings []Finding
	gateEnabled, ok := document.Scalars["gate.enabled"]
	if !ok {
		return nil, configError("relia.yaml missing required key gate.enabled")
	}
	switch gateEnabled.Value {
	case "false":
		if gateLimit, ok := document.Scalars["gate.max_error_recurrence_rate"]; ok {
			warnings = append(warnings, Finding{
				Type:    "unenforced_gate_setting",
				Message: "gate.max_error_recurrence_rate is configured while gate.enabled is false",
				Ref:     configRef(gateLimit),
			})
		}
	case "true":
		gateLimit, ok := document.Scalars["gate.max_error_recurrence_rate"]
		if !ok {
			return nil, configErrorAt("gate.max_error_recurrence_rate is required when gate.enabled is true", configRef(gateEnabled))
		}
		parsed, err := strconv.ParseFloat(gateLimit.Value, 64)
		if err != nil || parsed < 0 || parsed > 1 {
			return nil, configErrorAt("gate.max_error_recurrence_rate must be a number between 0 and 1 when gate.enabled is true", configRef(gateLimit))
		}
		warnings = append(warnings, Finding{
			Type:    "recurrence_gate_enabled",
			Message: "gate.enabled is true; relia backtest exits 5 when headline ERR exceeds the configured threshold",
			Ref:     configRef(gateEnabled),
		})
	default:
		return nil, configErrorAt("gate.enabled must be true or false", configRef(gateEnabled))
	}

	embeddings, ok := document.Scalars["distill.embeddings"]
	if !ok {
		return nil, configError("relia.yaml missing required key distill.embeddings")
	}
	switch embeddings.Value {
	case "signature":
	case "local":
		manifest := document.Scalars["models.local_manifest"]
		if commandErr := validateLocalModelManifest(root, manifest); commandErr != nil {
			return nil, commandErr
		}
	case "provider":
		return nil, dependencyError("provider embeddings require an approved model_provider_endpoint gate", configRef(embeddings))
	default:
		return nil, configError("distill.embeddings must be signature, local, or provider")
	}

	provider, ok := document.Scalars["distill.provider"]
	if !ok {
		return nil, configError("relia.yaml missing required key distill.provider")
	}
	switch provider.Value {
	case "none":
	case "openai_compatible", "anthropic":
		warnings = append(warnings, Finding{
			Type:    "provider_data_disclosure",
			Message: "provider-backed distill may send redacted experience records outside the machine when explicitly run",
			Ref:     configRef(provider),
		})
	default:
		return nil, configError("distill.provider must be none, openai_compatible, or anthropic")
	}

	reviewRequired, ok := document.Scalars["distill.review_required"]
	if !ok {
		return nil, configError("relia.yaml missing required key distill.review_required")
	}
	switch reviewRequired.Value {
	case "true":
	case "false":
		warnings = append(warnings, Finding{
			Type:    "review_gate_disabled",
			Message: "distill.review_required is disabled; drafted rules can bypass the default human review posture",
			Ref:     configRef(reviewRequired),
		})
	default:
		return nil, configError("distill.review_required must be true or false")
	}
	if len(yamlListValuesWithMapFields(document, "attribution.agent_authors", "login")) == 0 &&
		len(yamlListValues(document, "attribution.coauthor_trailers")) == 0 &&
		len(yamlListValues(document, "attribution.pr_labels")) == 0 {
		return nil, artifactContractError("attribution config has zero agent matchers; configure at least one agent_authors login, coauthor_trailer, or pr_label", yamlPathRef(document, "attribution"))
	}
	return warnings, nil
}

type localModelManifest struct {
	ModelID        string `json:"model_id"`
	Version        string `json:"version"`
	SourceURL      string `json:"source_url"`
	License        string `json:"license"`
	Digest         string `json:"digest"`
	CachePath      string `json:"cache_path"`
	UpdatePolicy   string `json:"update_policy"`
	RollbackPolicy string `json:"rollback_policy"`
}

func validateLocalModelManifest(root string, manifestScalar yamlScalar) *CommandError {
	manifestRel := strings.TrimSpace(manifestScalar.Value)
	if manifestRel == "" || filepath.IsAbs(manifestRel) {
		return dependencyError("local model manifest path must be repo-relative", configRef(manifestScalar))
	}
	manifestPath := filepath.Join(root, manifestRel)
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return dependencyError("local embedding artifact manifest is missing", configRef(manifestScalar))
		}
		return internalError("could not read local model manifest", err)
	}
	var manifest localModelManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return dependencyError("local model manifest is not valid JSON", manifestRel)
	}
	required := map[string]string{
		"model_id":        manifest.ModelID,
		"version":         manifest.Version,
		"source_url":      manifest.SourceURL,
		"license":         manifest.License,
		"digest":          manifest.Digest,
		"cache_path":      manifest.CachePath,
		"update_policy":   manifest.UpdatePolicy,
		"rollback_policy": manifest.RollbackPolicy,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return dependencyError("local model manifest missing required field "+field, manifestRel)
		}
	}
	if !strings.HasPrefix(manifest.SourceURL, "https://") {
		return dependencyError("local model manifest source_url must be https", manifestRel)
	}
	digest := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(manifest.Digest)), "sha256:")
	if len(digest) != 64 || !isHexDigest(digest) {
		return dependencyError("local model manifest digest must be a SHA-256 hex digest", manifestRel)
	}
	cachePath := filepath.Clean(manifest.CachePath)
	cachePathSlash := filepath.ToSlash(cachePath)
	if filepath.IsAbs(manifest.CachePath) || cachePath == "." || cachePath == ".." || strings.HasPrefix(cachePathSlash, "../") {
		return dependencyError("local model manifest cache_path must stay inside the repository", manifestRel)
	}
	if cachePathSlash == ".relia/models" || !strings.HasPrefix(cachePathSlash, ".relia/models/") {
		return dependencyError("local model manifest cache_path must stay under .relia/models", manifestRel)
	}
	artifactPath := filepath.Join(root, cachePath)
	artifactContent, err := os.ReadFile(artifactPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return dependencyError("local model artifact is missing", manifestRel)
		}
		return internalError("could not read local model artifact", err)
	}
	actual := fmt.Sprintf("%x", sha256.Sum256(artifactContent))
	if actual != digest {
		return dependencyError("local model artifact digest does not match manifest", manifestRel)
	}
	return nil
}

func isHexDigest(value string) bool {
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func validateSchemaContracts(root string) *CommandError {
	for _, rel := range requiredSchemaFiles {
		content, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return artifactContractError("required schema is missing: "+rel, rel)
			}
			return internalError("could not read schema "+rel, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(content, &schema); err != nil {
			return artifactContractError("schema is not valid JSON: "+rel, rel)
		}
		if schema["type"] != "object" {
			return artifactContractError("schema must describe a JSON object: "+rel, rel)
		}
		required, ok := schema["required"].([]any)
		if !ok {
			return artifactContractError("schema missing required array: "+rel, rel)
		}
		if !containsString(required, "schema_version") {
			return artifactContractError("schema must require schema_version: "+rel, rel)
		}
		if !containsString(required, "metadata") {
			return artifactContractError("schema must require metadata: "+rel, rel)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			return artifactContractError("schema missing properties object: "+rel, rel)
		}
		if _, ok := properties["schema_version"]; !ok {
			return artifactContractError("schema missing schema_version property: "+rel, rel)
		}
		if _, ok := properties["metadata"]; !ok {
			return artifactContractError("schema missing metadata property: "+rel, rel)
		}
	}
	return nil
}

func validateMemoryRuleArtifacts(root string) *CommandError {
	patterns := []string{
		filepath.Join(root, "memory", "rules", "*.yaml"),
		filepath.Join(root, "memory", "rules", "*.yml"),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return internalError("could not inspect memory rule artifacts", err)
		}
		for _, path := range matches {
			if commandErr := validateMemoryRuleArtifact(root, path); commandErr != nil {
				return commandErr
			}
		}
	}
	return nil
}

func validateMemoryRuleArtifact(root string, path string) *CommandError {
	content, err := os.ReadFile(path)
	if err != nil {
		return internalError("could not read memory rule artifact", err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	document, parseErr := parseYAMLDocument(string(content))
	if parseErr != nil {
		return artifactContractError(parseErr.Error(), rel)
	}
	required := []string{"object_type", "schema_version", "id", "kind", "status", "statement", "scope", "confidence", "evidence", "provenance", "review", "metadata"}
	for _, key := range required {
		if !hasYAMLPath(document, key) {
			return artifactContractError("memory rule missing required key "+key, rel)
		}
	}
	if document.Scalars["object_type"].Value != "relia.memory_rule" {
		return artifactContractError("memory rule object_type must be relia.memory_rule", configRefWithPath(rel, document.Scalars["object_type"]))
	}
	if document.Scalars["schema_version"].Value != commandSchemaVersion {
		return artifactContractError("memory rule schema_version must be "+commandSchemaVersion, rel)
	}
	kind := document.Scalars["kind"].Value
	if kind != "avoid" && kind != "playbook" {
		return artifactContractError("memory rule kind must be avoid or playbook", configRefWithPath(rel, document.Scalars["kind"]))
	}
	status := document.Scalars["status"].Value
	switch status {
	case "candidate", "active", "stale", "contradicted", "retired":
	default:
		return artifactContractError("memory rule status is invalid", configRefWithPath(rel, document.Scalars["status"]))
	}
	confidence, err := strconv.ParseFloat(document.Scalars["confidence"].Value, 64)
	if err != nil || confidence < 0 || confidence > 1 {
		return artifactContractError("memory rule confidence must be between 0 and 1", configRefWithPath(rel, document.Scalars["confidence"]))
	}
	if len(document.Lists["evidence.experiences"]) == 0 {
		return artifactContractError("memory rule must cite at least one experience", rel)
	}
	if len(document.Lists["scope.paths"]) == 0 && len(document.Lists["scope.signals"]) == 0 {
		return artifactContractError("memory rule must declare at least one scope path or signal", rel)
	}
	for _, scopePath := range document.Lists["scope.paths"] {
		if !repoPathExists(root, scopePath.Value) {
			return artifactContractError("memory rule scope path does not exist in the repo", configRefWithPath(rel, scopePath))
		}
	}
	evidenceCount, ok := document.Scalars["evidence.count"]
	if !ok {
		return artifactContractError("memory rule missing required key evidence.count", rel)
	}
	count, err := strconv.Atoi(evidenceCount.Value)
	if err != nil || count < 1 {
		return artifactContractError("memory rule evidence.count must be at least 1", configRefWithPath(rel, evidenceCount))
	}
	contradictionsScalar, ok := document.Scalars["evidence.contradictions"]
	if !ok {
		return artifactContractError("memory rule missing required key evidence.contradictions", rel)
	}
	contradictions, err := strconv.Atoi(contradictionsScalar.Value)
	if err != nil || contradictions < 0 {
		return artifactContractError("memory rule evidence.contradictions must be at least 0", configRefWithPath(rel, contradictionsScalar))
	}
	provenanceEntries := document.Lists["provenance"]
	if len(provenanceEntries) == 0 {
		return artifactContractError("memory rule must include at least one provenance entry", rel)
	}
	provenanceMaps := document.ListMaps["provenance"]
	if len(provenanceMaps) != len(provenanceEntries) {
		return artifactContractError("memory rule provenance entries must include pr and outcome", rel)
	}
	hasPositivePlaybookEvidence := false
	for _, provenance := range provenanceMaps {
		pr, ok := provenance["pr"]
		if !ok {
			return artifactContractError("memory rule provenance entry missing pr", rel)
		}
		prNumber, err := strconv.Atoi(pr.Value)
		if err != nil || prNumber < 1 {
			return artifactContractError("memory rule provenance pr must be at least 1", configRefWithPath(rel, pr))
		}
		outcome, ok := provenance["outcome"]
		if !ok {
			return artifactContractError("memory rule provenance entry missing outcome", rel)
		}
		switch outcome.Value {
		case "ci_failure", "revert", "review_correction", "fix_held", "merged_clean":
			if outcome.Value == "fix_held" || outcome.Value == "merged_clean" {
				hasPositivePlaybookEvidence = true
			}
		default:
			return artifactContractError("memory rule provenance outcome is invalid", configRefWithPath(rel, outcome))
		}
	}
	if kind == "playbook" && !hasPositivePlaybookEvidence {
		return artifactContractError("playbook memory rule must cite at least one fix_held or merged_clean provenance outcome", rel)
	}
	reviewLabel, ok := document.Scalars["review.label"]
	if !ok {
		return artifactContractError("memory rule missing required key review.label", rel)
	}
	switch reviewLabel.Value {
	case "accepted", "suggested", "needs_user_input":
	default:
		return artifactContractError("memory rule review.label is invalid", configRefWithPath(rel, reviewLabel))
	}
	if status == "active" && reviewLabel.Value != "accepted" {
		return artifactContractError("active memory rule review.label must be accepted", configRefWithPath(rel, reviewLabel))
	}
	statementOrigin, ok := document.Scalars["review.statement_origin"]
	if !ok {
		return artifactContractError("memory rule missing required key review.statement_origin", rel)
	}
	switch statementOrigin.Value {
	case "llm_drafted", "cluster_summary", "human_authored":
	default:
		return artifactContractError("memory rule review.statement_origin is invalid", configRefWithPath(rel, statementOrigin))
	}
	return nil
}

func repoPathExists(root string, rel string) bool {
	clean, ok := cleanRepoPath(rel)
	if !ok {
		return false
	}
	if workingTreePathMatches(root, clean) {
		return true
	}
	if output, err := exec.Command("git", "-C", root, "log", "--all", "--name-only", "--format=", "--", clean).Output(); err == nil {
		return strings.TrimSpace(string(output)) != ""
	}
	return false
}

func cleanRepoPath(rel string) (string, bool) {
	trimmed := strings.TrimSpace(rel)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return "", false
	}
	clean := filepath.Clean(trimmed)
	cleanSlash := filepath.ToSlash(clean)
	if clean == "." || clean == ".." || strings.HasPrefix(cleanSlash, "../") {
		return "", false
	}
	for _, part := range strings.Split(cleanSlash, "/") {
		if part == ".." {
			return "", false
		}
	}
	return clean, true
}

func workingTreePathMatches(root string, scope string) bool {
	scopeSlash := filepath.ToSlash(scope)
	if !hasGlobMagic(scopeSlash) {
		_, err := os.Stat(filepath.Join(root, scope))
		return err == nil
	}
	matched := false
	_ = filepath.WalkDir(root, func(candidate string, entry os.DirEntry, err error) error {
		if matched {
			return filepath.SkipAll
		}
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".factory", ".factoryd", ".relia":
				if candidate != root {
					return filepath.SkipDir
				}
			}
		}
		rel, err := filepath.Rel(root, candidate)
		if err != nil || rel == "." {
			return nil
		}
		if scopePatternMatches(scopeSlash, filepath.ToSlash(rel)) {
			matched = true
			return filepath.SkipAll
		}
		return nil
	})
	return matched
}

func hasGlobMagic(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

func scopePatternMatches(pattern string, rel string) bool {
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return rel == prefix || strings.HasPrefix(rel, prefix+"/")
	}
	matched, err := path.Match(pattern, rel)
	return err == nil && matched
}

func parseYAMLDocument(content string) (yamlDocument, error) {
	document := yamlDocument{
		Scalars:    map[string]yamlScalar{},
		Lists:      map[string][]yamlScalar{},
		ListMaps:   map[string][]map[string]yamlScalar{},
		Containers: map[string]yamlScalar{},
	}
	var stack []yamlContext
	blockScalarIndent := -1
	lines := strings.Split(content, "\n")
	for index, raw := range lines {
		lineNumber := index + 1
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := leadingSpaces(raw)
		if blockScalarIndent >= 0 {
			if indent > blockScalarIndent {
				continue
			}
			blockScalarIndent = -1
		}
		if indent%2 != 0 {
			return document, fmt.Errorf("invalid YAML indentation at line %d", lineNumber)
		}
		depth := indent / 2
		trimmed := stripInlineComment(strings.TrimSpace(raw))
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		if trimmed == "-" || strings.HasPrefix(trimmed, "- ") {
			if depth > len(stack) {
				return document, fmt.Errorf("list item without parent at line %d", lineNumber)
			}
			parent := yamlParentPath(stack, depth)
			if parent == "" {
				return document, fmt.Errorf("top-level lists are not supported at line %d", lineNumber)
			}
			itemValue := ""
			if strings.HasPrefix(trimmed, "- ") {
				itemValue = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			}
			document.Lists[parent] = append(document.Lists[parent], yamlScalar{
				Value: itemValue,
				Line:  lineNumber,
			})
			itemIndex := len(document.Lists[parent]) - 1
			stack = append(stack[:depth], yamlContext{
				Path:       fmt.Sprintf("%s[%d]", parent, itemIndex),
				ListItem:   true,
				ListParent: parent,
				ListIndex:  itemIndex,
			})
			if key, value, ok := cutYAMLMapping(itemValue); ok {
				scalarValue := unquoteScalar(value)
				recordListMapScalar(document, parent, itemIndex, key, yamlScalar{Value: scalarValue, Line: lineNumber})
				if scalarValue == ">" || scalarValue == "|" {
					blockScalarIndent = indent
				}
			}
			continue
		}
		key, value, ok := cutYAMLMapping(trimmed)
		if !ok {
			return document, fmt.Errorf("expected key/value pair at line %d", lineNumber)
		}
		if key == "" {
			return document, fmt.Errorf("empty key at line %d", lineNumber)
		}
		if depth > len(stack) {
			return document, fmt.Errorf("missing parent for %s at line %d", key, lineNumber)
		}
		path := key
		if parent := yamlParentPath(stack, depth); parent != "" {
			path = parent + "." + key
		}
		stack = append(stack[:depth], yamlContext{Path: path})
		if value == "" {
			document.Containers[path] = yamlScalar{Line: lineNumber}
			if listParent, listIndex, itemPath, ok := nearestListItem(stack[:depth]); ok {
				field := strings.TrimPrefix(path, itemPath+".")
				recordListMapScalar(document, listParent, listIndex, field, yamlScalar{Line: lineNumber})
			}
			continue
		}
		scalarValue := unquoteScalar(value)
		document.Scalars[path] = yamlScalar{Value: scalarValue, Line: lineNumber}
		if listParent, listIndex, itemPath, ok := nearestListItem(stack[:depth]); ok {
			field := strings.TrimPrefix(path, itemPath+".")
			recordListMapScalar(document, listParent, listIndex, field, yamlScalar{Value: scalarValue, Line: lineNumber})
		}
		if scalarValue == ">" || scalarValue == "|" {
			blockScalarIndent = indent
		}
	}
	return document, nil
}

func yamlParentPath(stack []yamlContext, depth int) string {
	if depth <= 0 {
		return ""
	}
	return stack[depth-1].Path
}

func cutYAMLMapping(value string) (string, string, bool) {
	key, rest, ok := strings.Cut(value, ":")
	if !ok {
		return "", "", false
	}
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	return key, strings.TrimSpace(rest), true
}

func nearestListItem(stack []yamlContext) (string, int, string, bool) {
	for index := len(stack) - 1; index >= 0; index-- {
		context := stack[index]
		if context.ListItem {
			return context.ListParent, context.ListIndex, context.Path, true
		}
	}
	return "", 0, "", false
}

func recordListMapScalar(document yamlDocument, parent string, index int, key string, scalar yamlScalar) {
	for len(document.ListMaps[parent]) <= index {
		document.ListMaps[parent] = append(document.ListMaps[parent], map[string]yamlScalar{})
	}
	document.ListMaps[parent][index][key] = scalar
}

func hasYAMLPath(document yamlDocument, path string) bool {
	if _, ok := document.Scalars[path]; ok {
		return true
	}
	if _, ok := document.Containers[path]; ok {
		return true
	}
	if _, ok := document.Lists[path]; ok {
		return true
	}
	if _, ok := document.ListMaps[path]; ok {
		return true
	}
	prefix := path + "."
	indexedPrefix := path + "["
	for key := range document.Scalars {
		if strings.HasPrefix(key, prefix) || strings.HasPrefix(key, indexedPrefix) {
			return true
		}
	}
	for key := range document.Containers {
		if strings.HasPrefix(key, prefix) || strings.HasPrefix(key, indexedPrefix) {
			return true
		}
	}
	for key := range document.Lists {
		if strings.HasPrefix(key, prefix) || strings.HasPrefix(key, indexedPrefix) {
			return true
		}
	}
	return false
}

func leadingSpaces(value string) int {
	count := 0
	for _, r := range value {
		if r != ' ' {
			return count
		}
		count++
	}
	return count
}

func stripInlineComment(value string) string {
	inSingle := false
	inDouble := false
	for index, r := range value {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble && (index == 0 || value[index-1] == ' ' || value[index-1] == '\t') {
				return strings.TrimSpace(value[:index])
			}
		}
	}
	return value
}

func unquoteScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func containsString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func configRef(scalar yamlScalar) string {
	return configRefWithPath(defaultConfigFile, scalar)
}

func configRefWithPath(path string, scalar yamlScalar) string {
	if scalar.Line <= 0 {
		return path
	}
	return fmt.Sprintf("%s:%d", path, scalar.Line)
}

func yamlPathRef(document yamlDocument, path string) string {
	if scalar, ok := document.Scalars[path]; ok {
		return configRef(scalar)
	}
	if scalar, ok := document.Containers[path]; ok {
		return configRef(scalar)
	}
	if scalars := document.Lists[path]; len(scalars) > 0 {
		return configRef(scalars[0])
	}
	prefix := path + "."
	bestLine := 0
	for _, collection := range []map[string]yamlScalar{document.Scalars, document.Containers} {
		for key, scalar := range collection {
			if strings.HasPrefix(key, prefix) && (bestLine == 0 || scalar.Line < bestLine) {
				bestLine = scalar.Line
			}
		}
	}
	if bestLine > 0 {
		return configRef(yamlScalar{Line: bestLine})
	}
	return defaultConfigFile
}

func defaultConfigYAML() string {
	return `version: 1

artifacts:
  schema_version: "1.0"
  relia_version: "0.0.0-dev"
  root: .relia
  commit_experiences: false

repo:
  provider: github
  remote: origin
  scopes: []

attribution:
  agent_authors: []
  coauthor_trailers:
    - Claude
    - Claude Code
  pr_labels:
    - agent-authored
  uncertain: exclude

outcomes:
  checks:
    required: []

privacy:
  local_only: true
  send_code: false
  send_diffs: false
  send_logs: false
  send_experience_records: false
  share_scope: private

redaction:
  schema_version: "1.0"
  entropy_scan: true
  fail_closed: true
  standard_token_shapes: true

distill:
  embeddings: signature
  provider: none
  review_required: true

models:
  local_manifest: .relia/models/manifest.json

serve:
  advisory_only: true

gate:
  enabled: false
`
}
