package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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
	defaultArtifactRoot     = ".relia"
	defaultMemoryRoot       = "memory"
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
	"schemas/relia-config.schema.json",
}

var artifactSkeletonDirs = []string{
	filepath.Join(defaultArtifactRoot, "experiences"),
	filepath.Join(defaultArtifactRoot, "signatures"),
	filepath.Join(defaultArtifactRoot, "coverage"),
	filepath.Join(defaultArtifactRoot, "reports"),
	filepath.Join(defaultArtifactRoot, "baselines"),
	filepath.Join(defaultMemoryRoot, "rules"),
	filepath.Join(defaultMemoryRoot, "compiled"),
}

const experienceGitignore = `# Relia experience shards are reproducible local caches by default.
/experiences/
`

var standardRedactionPatterns = []string{
	"api_key",
	"token",
	"password",
	"secret",
}

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
			"version":        "0.0.0-dev",
			"schema_version": commandSchemaVersion,
		})
	case "init":
		return initResult(parsed.commandArgs, start)
	case "check":
		return checkResult(parsed.commandArgs, start)
	case "models":
		if len(parsed.commandArgs) == 1 && parsed.commandArgs[0] == "pull" {
			return notImplementedResult("models pull", start)
		}
		return errorResult("models", "models", usageError("expected subcommand: pull"), start)
	case "ingest", "backtest", "distill", "review", "memory", "compile", "serve", "assess", "demo", "share":
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
			return errorResult("init", "init", internalError("could not create artifact skeleton", err), start)
		}
		if err := syncExperienceGitignore(root); err != nil {
			return errorResult("init", "init", internalError("could not sync experience cache ignore rule", err), start)
		}
		result := passResult("init", "init", "relia.yaml already exists", start, map[string]any{
			"config_path":   defaultConfigFile,
			"created":       false,
			"artifact_dirs": artifactSkeletonDirs,
		})
		result.Artifacts = append(result.Artifacts, artifact)
		return result
	} else if !errors.Is(err, os.ErrNotExist) {
		return errorResult("init", "init", internalError("could not inspect relia.yaml", err), start)
	}

	if err := os.WriteFile(configPath, []byte(defaultConfigYAML()), 0o644); err != nil {
		return errorResult("init", "init", internalError("could not write relia.yaml", err), start)
	}
	if err := ensureArtifactSkeleton(root); err != nil {
		return errorResult("init", "init", internalError("could not create artifact skeleton", err), start)
	}
	if err := syncExperienceGitignore(root); err != nil {
		return errorResult("init", "init", internalError("could not sync experience cache ignore rule", err), start)
	}
	result := passResult("init", "init", "created relia.yaml", start, map[string]any{
		"config_path":   defaultConfigFile,
		"created":       true,
		"artifact_dirs": artifactSkeletonDirs,
	})
	result.Artifacts = append(result.Artifacts, artifact, ArtifactRef{Kind: "ignore_rule", Path: filepath.ToSlash(filepath.Join(defaultArtifactRoot, ".gitignore"))})
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

	config, warnings, configErr := loadAndValidateConfig(root)
	if configErr != nil {
		return errorResult("check", "check", configErr, start)
	}
	schemas, schemaErr := validateSchemaContracts(root)
	if schemaErr != nil {
		return errorResult("check", "check", schemaErr, start)
	}
	rules, ruleErr := validateMemoryRules(root, config.MemoryRoot)
	if ruleErr != nil {
		return errorResult("check", "check", ruleErr, start)
	}

	result := passResult("check", "check", "local operating pack baseline is present", start, map[string]any{
		"checked_paths": len(requiredCheckFiles) + len(requiredSchemaFiles) + len(rules),
		"repo_root":     ".",
		"artifact_contract": map[string]any{
			"schema_version":          commandSchemaVersion,
			"generated_root":          config.ArtifactRoot,
			"user_memory_root":        config.MemoryRoot,
			"schemas":                 schemas,
			"validated_memory_rules":  rules,
			"generated_artifacts":     []string{".relia/experiences/*.jsonl", ".relia/signatures/index.json", ".relia/coverage/map.json", ".relia/reports/*", ".relia/baselines/err-baseline.json"},
			"user_approved_artifacts": []string{defaultConfigFile, "memory/rules/*.yaml", "memory/MEMORY.md", "memory/compiled/agents-block.md"},
		},
		"privacy_defaults": config.privacyData(),
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.Artifacts = append(result.Artifacts,
		ArtifactRef{Kind: "config", Path: defaultConfigFile},
		ArtifactRef{Kind: "schema", Path: "schemas/command-result.schema.json"},
		ArtifactRef{Kind: "schema", Path: "schemas/relia-config.schema.json"},
	)
	return result
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
	if commandErr.ExitCode == ExitRedactionSafety {
		result.RedactionStatus = "failed_closed"
	}
	result.Errors = append(result.Errors, *commandErr)
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
			"relia_version": reliaVersion,
			"schema_id":     "schemas/command-result.schema.json",
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

func validationError(message string, missing []string) *CommandError {
	return &CommandError{
		Type:        "operating_pack_validation_failed",
		Message:     message + ": " + strings.Join(missing, ", "),
		ExitCode:    ExitValidation,
		Remediation: "Restore the required repo lifecycle files before running Relia workflows.",
		Ref:         "docs/dev/dev_guides.md#validation-matrix",
	}
}

func artifactValidationError(message string, ref string) *CommandError {
	return &CommandError{
		Type:        "memory_artifact_validation_failed",
		Message:     message,
		ExitCode:    ExitValidation,
		Remediation: "Repair the schema, provenance, or artifact contract before running Relia workflows.",
		Ref:         ref,
	}
}

func redactionSafetyError(message string) *CommandError {
	return &CommandError{
		Type:        "redaction_safety_failed",
		Message:     message,
		ExitCode:    ExitRedactionSafety,
		Remediation: "Restore fail-closed redaction defaults before persisting or sharing artifacts.",
		Ref:         "docs/product/prd.md#redaction-pipeline-contract",
	}
}

func dependencyError(message string, ref string) *CommandError {
	return &CommandError{
		Type:        "dependency_error",
		Message:     message,
		ExitCode:    ExitDependency,
		Remediation: "Use embeddings: signature or run relia models pull after an approved model_artifact_pull gate.",
		Ref:         ref,
	}
}

func providerDependencyError(message string) *CommandError {
	return &CommandError{
		Type:        "dependency_error",
		Message:     message,
		ExitCode:    ExitDependency,
		Remediation: "Use embeddings: signature or complete the model_provider_endpoint grant before provider-backed distill work.",
		Ref:         "docs/dev/dev_guides.md#model-provider-and-artifact-policy",
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

type parsedYAML struct {
	scalars         map[string]string
	lists           map[string][]string
	listItemObjects map[string][]map[string]string
	objects         map[string]struct{}
	keys            map[string]struct{}
	children        map[string]map[string]struct{}
}

type reliaConfigSummary struct {
	ArtifactRoot        string
	MemoryRoot          string
	Embeddings          string
	ProviderConfigured  bool
	RedactionFailClosed bool
	EntropyScan         bool
	CommitExperiences   bool
	ShareScope          string
	OrgEligible         bool
	AdvisoryOnly        bool
}

func (config reliaConfigSummary) privacyData() map[string]any {
	return map[string]any{
		"redaction_fail_closed": config.RedactionFailClosed,
		"entropy_scan":          config.EntropyScan,
		"embeddings":            config.Embeddings,
		"commit_experiences":    config.CommitExperiences,
		"share_scope":           config.ShareScope,
		"org_eligible":          config.OrgEligible,
		"advisory_only":         config.AdvisoryOnly,
		"provider_configured":   config.ProviderConfigured,
	}
}

func ensureArtifactSkeleton(root string) error {
	for _, rel := range artifactSkeletonDirs {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func syncExperienceGitignore(root string) error {
	if shouldIgnoreExperienceShards(root) {
		return ensureExperienceGitignore(root)
	}
	return removeExperienceGitignore(root)
}

func ensureExperienceGitignore(root string) error {
	path := filepath.Join(root, defaultArtifactRoot, ".gitignore")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err == nil && strings.Contains(string(existing), "/experiences/") {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		if _, err := file.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = file.WriteString(experienceGitignore)
	return err
}

func removeExperienceGitignore(root string) error {
	path := filepath.Join(root, defaultArtifactRoot, ".gitignore")
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	existing := string(content)
	updated := strings.ReplaceAll(existing, experienceGitignore, "")
	if updated == existing {
		lines := strings.Split(existing, "\n")
		kept := make([]string, 0, len(lines))
		for index := 0; index < len(lines); index++ {
			if lines[index] == "# Relia experience shards are reproducible local caches by default." &&
				index+1 < len(lines) && lines[index+1] == "/experiences/" {
				index++
				continue
			}
			kept = append(kept, lines[index])
		}
		updated = strings.Join(kept, "\n")
	}
	if strings.TrimSpace(updated) == "" {
		return os.Remove(path)
	}
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func shouldIgnoreExperienceShards(root string) bool {
	content, err := os.ReadFile(filepath.Join(root, defaultConfigFile))
	if err != nil {
		return true
	}
	parsed, err := parseBaselineYAML(string(content))
	if err != nil {
		return true
	}
	commitExperiences, ok := parsed.boolScalar("memory.commit_experiences")
	return !ok || !commitExperiences
}

func loadAndValidateConfig(root string) (reliaConfigSummary, []Finding, *CommandError) {
	var summary reliaConfigSummary
	content, err := os.ReadFile(filepath.Join(root, defaultConfigFile))
	if err != nil {
		return summary, nil, configError("could not read relia.yaml")
	}
	parsed, parseErr := parseBaselineYAML(string(content))
	if parseErr != nil {
		return summary, nil, configError(parseErr.Error())
	}

	if parsed.scalar("version") != "1" {
		return summary, nil, configError("relia.yaml must declare version: 1")
	}
	var warnings []Finding
	if parsed.scalar("schema_version") == "" {
		applyPRDBootstrapDefaults(&parsed)
		warnings = append(warnings, Finding{
			Type:    "prd_bootstrap_config_normalized",
			Message: `relia.yaml omits schema_version; treating version: 1 as schema_version "1.0" and applying MVP-safe privacy/artifact defaults`,
			Ref:     "docs/product/prd.md#project-configuration",
		})
	} else if parsed.scalar("schema_version") != commandSchemaVersion {
		return summary, nil, configError(`relia.yaml must declare schema_version: "1.0"`)
	}
	artifactRoot := parsed.scalar("repo.artifact_root")
	if artifactRoot == "" {
		return summary, nil, configError("repo.artifact_root must be declared")
	}
	if artifactRoot != defaultArtifactRoot {
		return summary, nil, artifactValidationError("repo.artifact_root must remain "+defaultArtifactRoot+" in the MVP", defaultConfigFile)
	}
	memoryRoot := parsed.scalar("repo.memory_root")
	if memoryRoot == "" {
		return summary, nil, configError("repo.memory_root must be declared")
	}
	if memoryRoot != defaultMemoryRoot {
		return summary, nil, artifactValidationError("repo.memory_root must remain "+defaultMemoryRoot+" in the MVP", defaultConfigFile)
	}

	embeddings := parsed.scalar("distill.embeddings")
	if embeddings == "" {
		return summary, nil, configError("distill.embeddings is required")
	}
	if !containsString([]string{"signature", "local", "provider"}, embeddings) {
		return summary, nil, configError("distill.embeddings must be signature, local, or provider")
	}
	provider := parsed.scalar("distill.provider")
	if provider != "" && !containsString([]string{"anthropic", "openai-compatible"}, provider) {
		return summary, nil, configError("unknown distill.provider " + provider)
	}
	if embeddings == "local" {
		return summary, nil, dependencyError("local embedding artifact is missing", "docs/dev/dev_guides.md#model-provider-and-artifact-policy")
	}
	if embeddings == "provider" && provider == "" {
		return summary, nil, providerDependencyError("provider embeddings require distill.provider and a complete model_provider_endpoint grant")
	}
	if provider != "" {
		if missing := missingProviderEndpointGrantFields(parsed); len(missing) > 0 {
			return summary, nil, providerDependencyError("provider-backed distill work requires a complete model_provider_endpoint grant; missing " + strings.Join(missing, ", "))
		}
	}
	reviewRequired, ok := parsed.boolScalar("distill.review_required")
	if !ok {
		return summary, nil, configError("distill.review_required must be declared")
	}
	if !reviewRequired {
		return summary, nil, configError("distill.review_required must remain true in the MVP")
	}

	entropyScan, ok := parsed.boolScalar("redaction.entropy_scan")
	if !ok {
		return summary, nil, redactionSafetyError("redaction.entropy_scan must be declared")
	}
	failClosed, ok := parsed.boolScalar("redaction.fail_closed")
	if !ok {
		return summary, nil, redactionSafetyError("redaction.fail_closed must be declared")
	}
	if !entropyScan || !failClosed {
		return summary, nil, redactionSafetyError("redaction must keep entropy_scan and fail_closed enabled")
	}
	patterns := parsed.lists["redaction.patterns"]
	for _, required := range standardRedactionPatterns {
		if !containsString(patterns, required) {
			return summary, nil, redactionSafetyError("redaction.patterns must include standard token pattern " + required)
		}
	}

	commitExperiences, ok := parsed.boolScalar("memory.commit_experiences")
	if !ok {
		return summary, nil, configError("memory.commit_experiences must be declared")
	}
	shareScope := parsed.scalar("memory.share_scope")
	if shareScope == "" {
		return summary, nil, configError("memory.share_scope must be declared")
	}
	if shareScope != "private" {
		return summary, nil, redactionSafetyError("memory.share_scope must remain private in the MVP")
	}
	orgEligible, ok := parsed.boolScalar("memory.org_eligible")
	if !ok && !parsed.pathExists("memory.org_eligible") {
		return summary, nil, configError("memory.org_eligible must be declared")
	}
	if !ok || orgEligible {
		return summary, nil, redactionSafetyError("memory.org_eligible must remain false in the MVP")
	}
	advisoryOnly, ok := parsed.boolScalar("serve.advisory_only")
	if !ok {
		return summary, nil, configError("serve.advisory_only must be declared")
	}
	if !advisoryOnly {
		return summary, nil, configError("serve.advisory_only must remain true unless a later gate explicitly enables blocking behavior")
	}

	gateEnabled, gateEnabledSet := parsed.boolScalar("gate.enabled")
	if !gateEnabledSet {
		return summary, nil, configError("gate.enabled must be declared")
	}
	if !gateEnabled && parsed.scalar("gate.max_error_recurrence_rate") != "" {
		warnings = append(warnings, Finding{
			Type:    "declared_but_unenforceable_gate",
			Message: "gate.max_error_recurrence_rate is configured while gate.enabled is false",
			Ref:     defaultConfigFile,
		})
	}
	if commitExperiences {
		warnings = append(warnings, Finding{
			Type:    "experience_cache_commit_opt_in",
			Message: "memory.commit_experiences opts local experience shards into version control",
			Ref:     "docs/product/prd.md#core-artifacts",
		})
	}
	if provider != "" {
		warnings = append(warnings, Finding{
			Type:    "provider_path_configured",
			Message: "distill provider use must send only redacted records after an approved model_provider_endpoint gate",
			Ref:     "docs/architecture/architecture_guides.md#trust-mode-posture",
		})
	}
	if err := validateConfigAgainstSchema(root, parsed); err != nil {
		return summary, nil, err
	}

	return reliaConfigSummary{
		ArtifactRoot:        artifactRoot,
		MemoryRoot:          memoryRoot,
		Embeddings:          embeddings,
		ProviderConfigured:  provider != "",
		RedactionFailClosed: failClosed,
		EntropyScan:         entropyScan,
		CommitExperiences:   commitExperiences,
		ShareScope:          shareScope,
		OrgEligible:         orgEligible,
		AdvisoryOnly:        advisoryOnly,
	}, warnings, nil
}

func missingProviderEndpointGrantFields(parsed parsedYAML) []string {
	var missing []string
	for _, path := range []string{
		"distill.model",
		"distill.credential_env",
		"distill.budget_posture",
		"distill.redaction_posture",
	} {
		if parsed.scalar(path) == "" {
			missing = append(missing, path)
		}
	}
	if parsed.scalar("distill.endpoint") == "" && parsed.scalar("distill.base_url") == "" {
		missing = append(missing, "distill.endpoint_or_base_url")
	}
	if len(parsed.lists["distill.allowlist"]) == 0 {
		missing = append(missing, "distill.allowlist")
	}
	return missing
}

func validateSchemaContracts(root string) ([]string, *CommandError) {
	schemas := make([]string, 0, len(requiredSchemaFiles))
	for _, rel := range requiredSchemaFiles {
		content, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, artifactValidationError("required schema contract is missing: "+rel, rel)
		}
		var schema map[string]any
		if err := json.Unmarshal(content, &schema); err != nil {
			return nil, artifactValidationError("schema contract is not valid JSON: "+err.Error(), rel)
		}
		if schema["type"] != "object" {
			return nil, artifactValidationError("schema contract must declare type object", rel)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			return nil, artifactValidationError("schema contract must declare properties", rel)
		}
		if _, ok := properties["schema_version"]; !ok {
			return nil, artifactValidationError("schema contract must include schema_version property", rel)
		}
		if _, ok := properties["metadata"]; !ok {
			return nil, artifactValidationError("schema contract must include metadata property", rel)
		}
		required, ok := schema["required"].([]any)
		if !ok {
			return nil, artifactValidationError("schema contract must declare required fields", rel)
		}
		if !containsAnyString(required, "schema_version") || !containsAnyString(required, "metadata") {
			return nil, artifactValidationError("schema contract must require schema_version and metadata", rel)
		}
		if _, ok := schema["x-relia_error_mapping"].(map[string]any); !ok {
			return nil, artifactValidationError("schema contract must map validation errors to stable exits", rel)
		}
		schemas = append(schemas, rel)
	}
	return schemas, nil
}

func validateConfigAgainstSchema(root string, parsed parsedYAML) *CommandError {
	const rel = "schemas/relia-config.schema.json"
	content, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return artifactValidationError("required config schema contract is missing: "+rel, rel)
	}
	var schema map[string]any
	if err := json.Unmarshal(content, &schema); err != nil {
		return artifactValidationError("config schema contract is not valid JSON: "+err.Error(), rel)
	}
	if issue := validateYAMLPathAgainstSchema(parsed, "", schema); issue != nil {
		return configError("relia.yaml schema validation failed: " + issue.Error())
	}
	return nil
}

func applyPRDBootstrapDefaults(parsed *parsedYAML) {
	for _, entry := range []struct {
		path  string
		value string
	}{
		{"schema_version", commandSchemaVersion},
		{"repo.artifact_root", defaultArtifactRoot},
		{"repo.memory_root", defaultMemoryRoot},
		{"redaction.fail_closed", "true"},
		{"memory.share_scope", "private"},
		{"memory.org_eligible", "false"},
		{"serve.advisory_only", "true"},
		{"metadata.relia_version", reliaVersion},
		{"metadata.contract", "schemas/relia-config.schema.json"},
	} {
		if !parsed.pathExists(entry.path) {
			parsed.setScalar(entry.path, entry.value)
		}
	}
}

func validateMemoryRules(root string, memoryRoot string) ([]string, *CommandError) {
	pattern := filepath.Join(root, memoryRoot, "rules", "*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, internalError("could not inspect memory rules", err)
	}
	sort.Strings(matches)
	rules := make([]string, 0, len(matches))
	for _, path := range matches {
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil, internalError("could not resolve memory rule path", relErr)
		}
		rel = filepath.ToSlash(rel)
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, artifactValidationError("could not read memory rule", rel)
		}
		parsed, parseErr := parseBaselineYAML(string(content))
		if parseErr != nil {
			return nil, artifactValidationError("memory rule is not valid baseline YAML: "+parseErr.Error(), rel)
		}
		if validationErr := validateMemoryRuleContract(root, rel, parsed); validationErr != nil {
			return nil, validationErr
		}
		rules = append(rules, rel)
	}
	return rules, nil
}

func validateMemoryRuleContract(root string, rel string, parsed parsedYAML) *CommandError {
	if parsed.scalar("object_type") != "relia.memory_rule" {
		return artifactValidationError("memory rule object_type must be relia.memory_rule", rel)
	}
	if parsed.scalar("schema_version") != commandSchemaVersion {
		return artifactValidationError(`memory rule schema_version must be "1.0"`, rel)
	}
	if parsed.scalar("id") == "" {
		return artifactValidationError("memory rule id is required", rel)
	}
	if !containsString([]string{"avoid", "playbook"}, parsed.scalar("kind")) {
		return artifactValidationError("memory rule kind must be avoid or playbook", rel)
	}
	if parsed.scalar("status") == "" {
		return artifactValidationError("memory rule status is required", rel)
	}
	if !containsString([]string{"candidate", "active", "stale", "contradicted", "retired"}, parsed.scalar("status")) {
		return artifactValidationError("memory rule status is not valid", rel)
	}
	if parsed.scalar("statement") == "" {
		return artifactValidationError("memory rule statement is required", rel)
	}
	confidence, ok := parsed.numberScalar("confidence")
	if !ok || confidence < 0 || confidence > 1 {
		return artifactValidationError("memory rule confidence must be a number between 0 and 1", rel)
	}
	if !parsed.pathExists("evidence.count") {
		return artifactValidationError("memory rule evidence count is required", rel)
	}
	evidenceCount, err := strconv.Atoi(parsed.scalar("evidence.count"))
	if err != nil || evidenceCount < 1 {
		return artifactValidationError("memory rule evidence count must be at least 1", rel)
	}
	if len(parsed.lists["evidence.experiences"]) == 0 {
		return artifactValidationError("memory rule has no experience citations", rel)
	}
	if !parsed.pathExists("review.label") {
		return artifactValidationError("memory rule review label is required", rel)
	}
	if !containsString([]string{"accepted", "suggested", "needs_user_input", "rejected"}, parsed.scalar("review.label")) {
		return artifactValidationError("memory rule review label is not valid", rel)
	}
	if len(parsed.lists["scope.paths"]) == 0 && len(parsed.lists["scope.signals"]) == 0 {
		return artifactValidationError("memory rule scope must include paths or signals", rel)
	}
	for _, scopePath := range parsed.lists["scope.paths"] {
		if !memoryRuleScopePathExisted(root, scopePath) {
			return artifactValidationError("memory rule scope path never existed in repository history: "+scopePath, rel)
		}
	}
	if len(parsed.lists["provenance"]) == 0 {
		return artifactValidationError("memory rule has no provenance entries", rel)
	}
	provenanceObjects := parsed.listItemObjects["provenance"]
	if len(provenanceObjects) != len(parsed.lists["provenance"]) {
		return artifactValidationError("memory rule provenance pr is required", rel)
	}
	hasHeldOutcome := false
	for _, item := range provenanceObjects {
		if item["pr"] == "" {
			return artifactValidationError("memory rule provenance pr is required", rel)
		}
		outcome := item["outcome"]
		if outcome == "" {
			return artifactValidationError("memory rule provenance outcome is required", rel)
		}
		if !containsString([]string{"ci_failure", "revert", "review_correction", "merge_clean", "fix_held"}, outcome) {
			return artifactValidationError("memory rule provenance outcome is not valid", rel)
		}
		if outcome == "merge_clean" || outcome == "fix_held" {
			hasHeldOutcome = true
		}
	}
	if parsed.scalar("kind") == "playbook" && !hasHeldOutcome {
		return artifactValidationError("playbook memory rule must cite merge_clean or fix_held provenance", rel)
	}
	if parsed.scalar("metadata.relia_version") == "" {
		return artifactValidationError("memory rule metadata.relia_version is required", rel)
	}
	if parsed.scalar("status") == "active" {
		label := parsed.scalar("review.label")
		if label == "" {
			return artifactValidationError("active rule has no accepted review label", rel)
		}
		if label != "accepted" {
			return artifactValidationError("active rule review label must be accepted", rel)
		}
	}
	return nil
}

func memoryRuleScopePathExisted(root string, rawScopePath string) bool {
	scopePath := normalizeScopePath(rawScopePath)
	if scopePath == "" || scopePath == "." || strings.HasPrefix(scopePath, "../") {
		return false
	}
	if currentTreeHasScopePath(root, scopePath) {
		return true
	}
	return gitHistoryHasScopePath(root, scopePath)
}

func normalizeScopePath(rawScopePath string) string {
	scopePath := strings.TrimSpace(filepath.ToSlash(rawScopePath))
	scopePath = strings.TrimPrefix(scopePath, "./")
	if path.IsAbs(scopePath) {
		return ""
	}
	return path.Clean(scopePath)
}

func currentTreeHasScopePath(root string, scopePath string) bool {
	literalPrefix := scopePathLiteralPrefix(scopePath)
	if !scopePathHasPattern(scopePath) && literalPrefix != "" {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(literalPrefix))); err == nil {
			return true
		}
	}

	walkErr := filepath.WalkDir(root, func(candidate string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, candidate)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if entry.IsDir() && (rel == ".git" || rel == ".factoryd" || rel == ".factory/tmp") {
			return filepath.SkipDir
		}
		if scopePathMatches(scopePath, rel) {
			return errScopePathMatched
		}
		return nil
	})
	return errors.Is(walkErr, errScopePathMatched)
}

func gitHistoryHasScopePath(root string, scopePath string) bool {
	pathspec := scopePath
	if scopePathHasPattern(scopePath) {
		pathspec = scopePathLiteralPrefix(scopePath)
	}
	args := []string{"-C", root, "log", "--all", "--name-only", "--pretty=format:", "--"}
	if pathspec != "" {
		args = append(args, pathspec)
	}
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(output), "\n") {
		rel := strings.TrimSpace(filepath.ToSlash(line))
		if rel != "" && scopePathMatches(scopePath, rel) {
			return true
		}
	}
	return false
}

func scopePathHasPattern(scopePath string) bool {
	return strings.ContainsAny(scopePath, "*?[")
}

func scopePathLiteralPrefix(scopePath string) string {
	prefix := scopePath
	if index := strings.IndexAny(scopePath, "*?["); index >= 0 {
		prefix = scopePath[:index]
	}
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" {
		return ""
	}
	return path.Clean(prefix)
}

func scopePathMatches(scopePath string, rel string) bool {
	if !scopePathHasPattern(scopePath) {
		return rel == scopePath || strings.HasPrefix(rel, scopePath+"/")
	}
	if strings.HasSuffix(scopePath, "/**") {
		prefix := strings.TrimSuffix(scopePath, "/**")
		return rel == prefix || strings.HasPrefix(rel, prefix+"/")
	}
	matched, err := path.Match(scopePath, rel)
	return err == nil && matched
}

var errScopePathMatched = errors.New("scope path matched")

type schemaValidationIssue struct {
	path    string
	message string
}

func (issue schemaValidationIssue) Error() string {
	if issue.path == "" {
		return issue.message
	}
	return issue.path + " " + issue.message
}

func validateYAMLPathAgainstSchema(parsed parsedYAML, path string, schema map[string]any) *schemaValidationIssue {
	if path != "" && !parsed.pathExists(path) {
		return nil
	}
	if wantType, ok := schema["type"].(string); ok {
		if issue := validateYAMLType(parsed, path, wantType); issue != nil {
			return issue
		}
	}
	if schemaAllOf, ok := schema["allOf"].([]any); ok {
		for _, rawClause := range schemaAllOf {
			clause, ok := rawClause.(map[string]any)
			if !ok {
				continue
			}
			if schemaConditionalMatches(parsed, path, clause["if"]) {
				thenSchema, ok := clause["then"].(map[string]any)
				if !ok {
					continue
				}
				if issue := validateYAMLPathAgainstSchema(parsed, path, thenSchema); issue != nil {
					return issue
				}
			}
		}
	}
	if schemaAnyOf, ok := schema["anyOf"].([]any); ok {
		matched := false
		var firstIssue *schemaValidationIssue
		for _, rawOption := range schemaAnyOf {
			option, ok := rawOption.(map[string]any)
			if !ok {
				continue
			}
			if issue := validateYAMLPathAgainstSchema(parsed, path, option); issue != nil {
				if firstIssue == nil {
					firstIssue = issue
				}
				continue
			}
			matched = true
			break
		}
		if !matched {
			if firstIssue != nil {
				return firstIssue
			}
			return &schemaValidationIssue{path: path, message: "must match at least one schema option"}
		}
	}
	if wantConst, ok := schema["const"]; ok {
		if !parsed.scalarMatches(path, wantConst) {
			return &schemaValidationIssue{path: path, message: "must equal " + schemaValueForMessage(wantConst)}
		}
	}
	if enumValues, ok := schema["enum"].([]any); ok {
		matched := false
		for _, enumValue := range enumValues {
			if parsed.scalarMatches(path, enumValue) {
				matched = true
				break
			}
		}
		if !matched {
			return &schemaValidationIssue{path: path, message: "must be one of " + schemaValueForMessage(enumValues)}
		}
	}
	if minimum, ok := schema["minimum"].(float64); ok {
		value, ok := parsed.numberScalar(path)
		if !ok {
			return &schemaValidationIssue{path: path, message: "must be a number"}
		}
		if value < minimum {
			return &schemaValidationIssue{path: path, message: "must be at least " + schemaValueForMessage(minimum)}
		}
	}
	if maximum, ok := schema["maximum"].(float64); ok {
		value, ok := parsed.numberScalar(path)
		if !ok {
			return &schemaValidationIssue{path: path, message: "must be a number"}
		}
		if value > maximum {
			return &schemaValidationIssue{path: path, message: "must be at most " + schemaValueForMessage(maximum)}
		}
	}
	if minLength, ok := schema["minLength"].(float64); ok {
		value, ok := parsed.scalars[path]
		if !ok || len(value) < int(minLength) {
			return &schemaValidationIssue{path: path, message: "must have length at least " + strconv.Itoa(int(minLength))}
		}
	}
	if minItems, ok := schema["minItems"].(float64); ok {
		values, ok := parsed.lists[path]
		if !ok || len(values) < int(minItems) {
			return &schemaValidationIssue{path: path, message: "must have at least " + strconv.Itoa(int(minItems)) + " items"}
		}
	}
	if itemsSchema, ok := schema["items"].(map[string]any); ok {
		if issue := validateYAMLArrayItems(parsed, path, itemsSchema); issue != nil {
			return issue
		}
	}
	_, hasRequiredFields := schema["required"]
	_, hasAdditionalProperties := schema["additionalProperties"]
	if schema["type"] == "object" || len(schemaObjectProperties(schema)) > 0 || hasRequiredFields || hasAdditionalProperties {
		return validateYAMLObjectAgainstSchema(parsed, path, schema)
	}
	return nil
}

func schemaConditionalMatches(parsed parsedYAML, path string, rawSchema any) bool {
	schema, ok := rawSchema.(map[string]any)
	return ok && validateYAMLPathAgainstSchema(parsed, path, schema) == nil
}

func validateYAMLType(parsed parsedYAML, path string, wantType string) *schemaValidationIssue {
	switch wantType {
	case "object":
		if !parsed.isObject(path) {
			return &schemaValidationIssue{path: path, message: "must be an object"}
		}
	case "array":
		if _, ok := parsed.lists[path]; !ok {
			return &schemaValidationIssue{path: path, message: "must be an array"}
		}
	case "string":
		if _, ok := parsed.scalars[path]; !ok {
			return &schemaValidationIssue{path: path, message: "must be a string"}
		}
	case "boolean":
		if _, ok := parsed.boolScalar(path); !ok {
			return &schemaValidationIssue{path: path, message: "must be a boolean"}
		}
	case "integer":
		value, ok := parsed.scalars[path]
		if !ok {
			return &schemaValidationIssue{path: path, message: "must be an integer"}
		}
		if _, err := strconv.Atoi(value); err != nil {
			return &schemaValidationIssue{path: path, message: "must be an integer"}
		}
	case "number":
		value, ok := parsed.scalars[path]
		if !ok {
			return &schemaValidationIssue{path: path, message: "must be a number"}
		}
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return &schemaValidationIssue{path: path, message: "must be a number"}
		}
	}
	return nil
}

func validateYAMLArrayItems(parsed parsedYAML, path string, itemsSchema map[string]any) *schemaValidationIssue {
	values, ok := parsed.lists[path]
	if !ok {
		return nil
	}
	itemType, _ := itemsSchema["type"].(string)
	minLength, hasMinLength := itemsSchema["minLength"].(float64)
	for index, value := range values {
		itemPath := path + "[" + strconv.Itoa(index) + "]"
		if itemType == "string" && value == "" {
			return &schemaValidationIssue{path: itemPath, message: "must be a string"}
		}
		if hasMinLength && len(value) < int(minLength) {
			return &schemaValidationIssue{path: itemPath, message: "must have length at least " + strconv.Itoa(int(minLength))}
		}
	}
	return nil
}

func validateYAMLObjectAgainstSchema(parsed parsedYAML, path string, schema map[string]any) *schemaValidationIssue {
	properties := schemaObjectProperties(schema)
	if required, ok := schema["required"].([]any); ok {
		for _, rawName := range required {
			name, ok := rawName.(string)
			if !ok {
				continue
			}
			childPath := joinYAMLPath(path, name)
			if !parsed.pathExists(childPath) {
				return &schemaValidationIssue{path: childPath, message: "is required"}
			}
		}
	}
	if allowAdditional, ok := schema["additionalProperties"].(bool); ok && !allowAdditional {
		childKeys := parsed.childKeys(path)
		for _, childKey := range childKeys {
			if _, ok := properties[childKey]; !ok {
				return &schemaValidationIssue{path: joinYAMLPath(path, childKey), message: "is not allowed by the schema"}
			}
		}
	}
	propertyNames := make([]string, 0, len(properties))
	for name := range properties {
		propertyNames = append(propertyNames, name)
	}
	sort.Strings(propertyNames)
	for _, name := range propertyNames {
		childPath := joinYAMLPath(path, name)
		if !parsed.pathExists(childPath) {
			continue
		}
		childSchema, ok := properties[name].(map[string]any)
		if !ok {
			continue
		}
		if issue := validateYAMLPathAgainstSchema(parsed, childPath, childSchema); issue != nil {
			return issue
		}
	}
	return nil
}

func schemaObjectProperties(schema map[string]any) map[string]any {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return properties
}

func joinYAMLPath(parent string, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func schemaValueForMessage(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

type yamlBlockScalar struct {
	path   string
	indent int
	lines  []string
}

func (block yamlBlockScalar) active() bool {
	return block.path != ""
}

func (block *yamlBlockScalar) appendLine(line string) {
	if len(line) >= block.indent {
		block.lines = append(block.lines, line[block.indent:])
		return
	}
	block.lines = append(block.lines, strings.TrimLeft(line, " "))
}

func flushYAMLBlockScalar(parsed *parsedYAML, block yamlBlockScalar) {
	if !block.active() {
		return
	}
	lines := append([]string(nil), block.lines...)
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	parsed.setScalar(block.path, strings.TrimSpace(strings.Join(lines, "\n")))
}

func isYAMLBlockScalarIndicator(value string) bool {
	switch value {
	case ">", "|", ">-", "|-", ">+", "|+":
		return true
	default:
		return false
	}
}

func parseBaselineYAML(content string) (parsedYAML, error) {
	parsed := parsedYAML{
		scalars:         map[string]string{},
		lists:           map[string][]string{},
		listItemObjects: map[string][]map[string]string{},
		objects:         map[string]struct{}{},
		keys:            map[string]struct{}{},
		children:        map[string]map[string]struct{}{},
	}
	var section string
	var listPath string
	var listItemIndent int
	var blockScalar yamlBlockScalar
	for lineNumber, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if blockScalar.active() {
			if strings.TrimSpace(line) == "" {
				if len(blockScalar.lines) > 0 {
					blockScalar.lines = append(blockScalar.lines, "")
				}
				continue
			}
			indent := len(line) - len(strings.TrimLeft(line, " "))
			if indent >= blockScalar.indent {
				blockScalar.appendLine(line)
				continue
			}
			flushYAMLBlockScalar(&parsed, blockScalar)
			blockScalar = yamlBlockScalar{}
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		trimmed := stripYAMLComment(strings.TrimSpace(line))
		if trimmed == "" {
			continue
		}
		if listPath != "" && indent > listItemIndent {
			if key, value, ok := splitYAMLKeyValue(trimmed); ok && len(parsed.listItemObjects[listPath]) > 0 {
				lastIndex := len(parsed.listItemObjects[listPath]) - 1
				parsed.listItemObjects[listPath][lastIndex][key] = value
			}
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			if listPath == "" {
				if indent >= 4 {
					continue
				}
				return parsed, fmt.Errorf("line %d: list item has no parent key", lineNumber+1)
			}
			if indent != listItemIndent {
				if indent > listItemIndent {
					continue
				}
				listPath = ""
				listItemIndent = 0
				if indent >= 4 {
					continue
				}
				return parsed, fmt.Errorf("line %d: list item has no parent key", lineNumber+1)
			}
			delete(parsed.objects, listPath)
			item := cleanYAMLScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			parsed.lists[listPath] = append(parsed.lists[listPath], item)
			itemObject := map[string]string{}
			if key, value, ok := splitYAMLKeyValue(item); ok {
				itemObject[key] = value
			}
			parsed.listItemObjects[listPath] = append(parsed.listItemObjects[listPath], itemObject)
			continue
		}
		key, rawValue, ok := strings.Cut(trimmed, ":")
		if !ok {
			return parsed, fmt.Errorf("line %d: expected key: value", lineNumber+1)
		}
		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)
		inlineList, hasInlineList, inlineListErr := parseInlineYAMLSequence(rawValue)
		if inlineListErr != nil {
			return parsed, fmt.Errorf("line %d: %w", lineNumber+1, inlineListErr)
		}
		value := cleanYAMLScalar(rawValue)
		if key == "" {
			return parsed, fmt.Errorf("line %d: empty key", lineNumber+1)
		}
		listPath = ""
		listItemIndent = 0
		switch {
		case indent == 0:
			section = key
			parsed.keys[key] = struct{}{}
			if rawValue != "" {
				if hasInlineList {
					parsed.lists[key] = inlineList
				} else if isYAMLBlockScalarIndicator(rawValue) {
					blockScalar = yamlBlockScalar{path: key, indent: indent + 2}
				} else {
					parsed.scalars[key] = value
				}
			} else {
				parsed.objects[key] = struct{}{}
				listPath = key
				listItemIndent = indent + 2
			}
		case indent == 2 && section != "":
			path := section + "." + key
			parsed.keys[path] = struct{}{}
			if parsed.children[section] == nil {
				parsed.children[section] = map[string]struct{}{}
			}
			parsed.children[section][key] = struct{}{}
			if rawValue == "" {
				listPath = path
				listItemIndent = indent + 2
				parsed.objects[path] = struct{}{}
			} else if hasInlineList {
				parsed.lists[path] = inlineList
			} else if isYAMLBlockScalarIndicator(rawValue) {
				blockScalar = yamlBlockScalar{path: path, indent: indent + 2}
			} else {
				parsed.scalars[path] = value
			}
		default:
			// The baseline validator only consumes top-level fields and
			// first-level section keys. Deeper MVP config is validated by the
			// command that owns that behavior.
			continue
		}
	}
	flushYAMLBlockScalar(&parsed, blockScalar)
	return parsed, nil
}

func (parsed parsedYAML) scalar(path string) string {
	return parsed.scalars[path]
}

func (parsed parsedYAML) boolScalar(path string) (bool, bool) {
	value, ok := parsed.scalars[path]
	if !ok {
		return false, false
	}
	switch value {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func (parsed parsedYAML) pathExists(path string) bool {
	if path == "" {
		return true
	}
	if _, ok := parsed.keys[path]; ok {
		return true
	}
	if _, ok := parsed.scalars[path]; ok {
		return true
	}
	if _, ok := parsed.lists[path]; ok {
		return true
	}
	if _, ok := parsed.objects[path]; ok {
		return true
	}
	return false
}

func (parsed *parsedYAML) setScalar(path string, value string) {
	if parsed.scalars == nil {
		parsed.scalars = map[string]string{}
	}
	if parsed.keys == nil {
		parsed.keys = map[string]struct{}{}
	}
	if parsed.children == nil {
		parsed.children = map[string]map[string]struct{}{}
	}
	if parsed.objects == nil {
		parsed.objects = map[string]struct{}{}
	}
	if parsed.lists == nil {
		parsed.lists = map[string][]string{}
	}
	if parsed.listItemObjects == nil {
		parsed.listItemObjects = map[string][]map[string]string{}
	}
	parsed.scalars[path] = value
	parsed.keys[path] = struct{}{}
	delete(parsed.lists, path)
	delete(parsed.listItemObjects, path)
	delete(parsed.objects, path)
	if parent, child, ok := strings.Cut(path, "."); ok {
		parsed.keys[parent] = struct{}{}
		parsed.objects[parent] = struct{}{}
		if parsed.children[parent] == nil {
			parsed.children[parent] = map[string]struct{}{}
		}
		parsed.children[parent][child] = struct{}{}
	}
}

func (parsed parsedYAML) isObject(path string) bool {
	if path == "" {
		return true
	}
	_, ok := parsed.objects[path]
	return ok
}

func (parsed parsedYAML) childKeys(path string) []string {
	keys := []string{}
	if path == "" {
		for key := range parsed.keys {
			if !strings.Contains(key, ".") {
				keys = append(keys, key)
			}
		}
	} else {
		for key := range parsed.children[path] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func (parsed parsedYAML) scalarMatches(path string, want any) bool {
	switch typed := want.(type) {
	case string:
		return parsed.scalar(path) == typed
	case bool:
		value, ok := parsed.boolScalar(path)
		return ok && value == typed
	case float64:
		value, ok := parsed.scalars[path]
		if !ok {
			return false
		}
		parsedNumber, err := strconv.ParseFloat(value, 64)
		return err == nil && parsedNumber == typed
	default:
		return false
	}
}

func (parsed parsedYAML) numberScalar(path string) (float64, bool) {
	value, ok := parsed.scalars[path]
	if !ok {
		return 0, false
	}
	number, err := strconv.ParseFloat(value, 64)
	return number, err == nil
}

func stripYAMLComment(value string) string {
	inSingleQuote := false
	inDoubleQuote := false
	for index, char := range value {
		switch char {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '#':
			if !inSingleQuote && !inDoubleQuote {
				return strings.TrimSpace(value[:index])
			}
		}
	}
	return strings.TrimSpace(value)
}

func splitYAMLKeyValue(value string) (string, string, bool) {
	key, rawValue, ok := strings.Cut(value, ":")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	return key, cleanYAMLScalar(rawValue), true
}

func parseInlineYAMLSequence(value string) ([]string, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false, nil
	}
	if !strings.HasPrefix(value, "[") && !strings.HasSuffix(value, "]") {
		return nil, false, nil
	}
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, false, errors.New("inline sequence must be enclosed in []")
	}
	body := strings.TrimSpace(value[1 : len(value)-1])
	if body == "" {
		return []string{}, true, nil
	}

	var items []string
	var token strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	for _, char := range body {
		if escaped {
			token.WriteRune(char)
			escaped = false
			continue
		}
		if inDoubleQuote && char == '\\' {
			token.WriteRune(char)
			escaped = true
			continue
		}
		switch char {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
			token.WriteRune(char)
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
			token.WriteRune(char)
		case ',':
			if inSingleQuote || inDoubleQuote {
				token.WriteRune(char)
				continue
			}
			items = append(items, cleanYAMLScalar(token.String()))
			token.Reset()
		default:
			token.WriteRune(char)
		}
	}
	if inSingleQuote || inDoubleQuote {
		return nil, false, errors.New("inline sequence has an unterminated quoted scalar")
	}
	items = append(items, cleanYAMLScalar(token.String()))
	return items, true, nil
}

func cleanYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsAnyString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func renderAndExit(stdout io.Writer, stderr io.Writer, result CommandResult, flags globalFlags, stdoutIsTTY bool) int {
	machineReadable := flags.json || flags.quiet || flags.compact || !stdoutIsTTY
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

func defaultConfigYAML() string {
	return `version: 1
schema_version: "1.0"

repo:
  provider: github
  remote: origin
  artifact_root: .relia
  memory_root: memory
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
  revert_detection: true
  lookback_days: 180

redaction:
  patterns:
    - api_key
    - token
    - password
    - secret
    - private_key
  entropy_scan: true
  fail_closed: true

distill:
  embeddings: signature
  review_required: true

memory:
  decay_half_life_days: 90
  invalidate_on_path_delete: true
  max_active_rules: 200
  commit_experiences: false
  share_scope: private
  org_eligible: false

serve:
  advisory_only: true

advise:
  enabled: false
  max_comments_per_pr: 1
  update_in_place: true
  reassess_debounce_minutes: 10
  min_confidence: 0.6

badge:
  stale_after_days: 30
  stale_after_merged_prs: 20

gate:
  enabled: false

metadata:
  relia_version: 0.0.0-dev
  contract: schemas/relia-config.schema.json
`
}
