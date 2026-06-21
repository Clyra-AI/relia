package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	filepath.Join(".relia", "experiences"),
	filepath.Join(".relia", "signatures"),
	filepath.Join(".relia", "coverage"),
	filepath.Join(".relia", "reports"),
	filepath.Join(".relia", "baselines"),
	filepath.Join("memory", "rules"),
	filepath.Join("memory", "compiled"),
}

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
	result := passResult("init", "init", "created relia.yaml", start, map[string]any{
		"config_path":   defaultConfigFile,
		"created":       true,
		"artifact_dirs": artifactSkeletonDirs,
	})
	result.Artifacts = append(result.Artifacts, artifact)
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

	result := passResult("check", "check", "local operating pack baseline is present", start, map[string]any{
		"checked_paths": len(requiredCheckFiles) + len(requiredSchemaFiles),
		"repo_root":     ".",
		"artifact_contract": map[string]any{
			"schema_version":          commandSchemaVersion,
			"generated_root":          ".relia",
			"user_memory_root":        "memory",
			"schemas":                 schemas,
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
	scalars map[string]string
	lists   map[string][]string
}

type reliaConfigSummary struct {
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
	if parsed.scalar("schema_version") != commandSchemaVersion {
		return summary, nil, configError(`relia.yaml must declare schema_version: "1.0"`)
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
		return summary, nil, dependencyError("provider embeddings require distill.provider", defaultConfigFile)
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
		return summary, nil, artifactValidationError("memory.share_scope must remain private in the MVP", defaultConfigFile)
	}
	orgEligible, ok := parsed.boolScalar("memory.org_eligible")
	if !ok {
		return summary, nil, configError("memory.org_eligible must be declared")
	}
	if orgEligible {
		return summary, nil, artifactValidationError("memory.org_eligible must remain false in the MVP", defaultConfigFile)
	}
	advisoryOnly, ok := parsed.boolScalar("serve.advisory_only")
	if !ok {
		return summary, nil, configError("serve.advisory_only must be declared")
	}
	if !advisoryOnly {
		return summary, nil, configError("serve.advisory_only must remain true unless a later gate explicitly enables blocking behavior")
	}

	var warnings []Finding
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

	return reliaConfigSummary{
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

func parseBaselineYAML(content string) (parsedYAML, error) {
	parsed := parsedYAML{
		scalars: map[string]string{},
		lists:   map[string][]string{},
	}
	var section string
	var listPath string
	for lineNumber, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		trimmed := stripYAMLComment(strings.TrimSpace(line))
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			if listPath == "" {
				return parsed, fmt.Errorf("line %d: list item has no parent key", lineNumber+1)
			}
			parsed.lists[listPath] = append(parsed.lists[listPath], cleanYAMLScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return parsed, fmt.Errorf("line %d: expected key: value", lineNumber+1)
		}
		key = strings.TrimSpace(key)
		value = cleanYAMLScalar(strings.TrimSpace(value))
		if key == "" {
			return parsed, fmt.Errorf("line %d: empty key", lineNumber+1)
		}
		listPath = ""
		switch {
		case indent == 0:
			section = key
			if value != "" {
				parsed.scalars[key] = value
			}
		case indent == 2 && section != "":
			path := section + "." + key
			if value == "" {
				listPath = path
				if _, ok := parsed.lists[path]; !ok {
					parsed.lists[path] = []string{}
				}
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

gate:
  enabled: false

metadata:
  relia_version: 0.0.0-dev
  contract: schemas/relia-config.schema.json
`
}
