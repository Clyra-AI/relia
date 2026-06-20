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
	Kind          string `json:"kind"`
	Path          string `json:"path"`
	SchemaVersion string `json:"schema_version"`
}

type configSummary struct {
	GeneratedArtifactsRoot string
	MemoryArtifactsRoot    string
	DefaultShareScope      string
	SchemaVersion          string
	DistillProvider        string
	DistillEmbeddings      string
	RedactionEntropyScan   bool
	CommitExperiences      bool
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
	artifact := ArtifactRef{Kind: "config", Path: defaultConfigFile, SchemaVersion: commandSchemaVersion}
	if _, err := os.Stat(configPath); err == nil {
		result := passResult("init", "init", "relia.yaml already exists", start, map[string]any{
			"config_path": defaultConfigFile,
			"created":     false,
		})
		result.Artifacts = append(result.Artifacts, artifact)
		return result
	} else if !errors.Is(err, os.ErrNotExist) {
		return errorResult("init", "init", internalError("could not inspect relia.yaml", err), start)
	}

	if err := os.WriteFile(configPath, []byte(defaultConfigYAML()), 0o644); err != nil {
		return errorResult("init", "init", internalError("could not write relia.yaml", err), start)
	}
	result := passResult("init", "init", "created relia.yaml", start, map[string]any{
		"config_path": defaultConfigFile,
		"created":     true,
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

	if commandErr := validateSchemaContracts(root); commandErr != nil {
		return errorResult("check", "check", commandErr, start)
	}
	config, warnings, commandErr := loadAndValidateConfig(root)
	if commandErr != nil {
		return errorResult("check", "check", commandErr, start)
	}
	if commandErr := validateMemoryRules(root); commandErr != nil {
		return errorResult("check", "check", commandErr, start)
	}

	result := passResult("check", "check", "local operating pack baseline is present", start, map[string]any{
		"checked_paths":             len(requiredCheckFiles),
		"schema_contract_count":     len(requiredSchemaFiles),
		"repo_root":                 ".",
		"generated_artifacts_root":  config.GeneratedArtifactsRoot,
		"memory_artifacts_root":     config.MemoryArtifactsRoot,
		"default_share_scope":       config.DefaultShareScope,
		"commit_experiences":        config.CommitExperiences,
		"redaction_entropy_scan":    config.RedactionEntropyScan,
		"distill_embeddings":        config.DistillEmbeddings,
		"live_provider_configured":  config.DistillProvider != "",
		"artifact_schema_version":   config.SchemaVersion,
		"relia_version":             "0.0.0-dev",
		"privacy_default_posture":   "local_private_redacted",
		"required_schema_contracts": requiredSchemaFiles,
	})
	result.Warnings = append(result.Warnings, warnings...)
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
		Data:            data,
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

func dependencyError(message string, ref string) *CommandError {
	return &CommandError{
		Type:        "dependency_error",
		Message:     message,
		ExitCode:    ExitDependency,
		Remediation: "Run relia models pull with an approved model_artifact_pull gate or use embeddings: signature.",
		Ref:         ref,
	}
}

func schemaContractError(message string, ref string) *CommandError {
	return &CommandError{
		Type:        "schema_contract_validation_failed",
		Message:     message,
		ExitCode:    ExitValidation,
		Remediation: "Repair the versioned schema or dependent Relia artifact before running Relia workflows.",
		Ref:         ref,
	}
}

func memoryArtifactError(message string, ref string) *CommandError {
	return &CommandError{
		Type:        "memory_artifact_validation_failed",
		Message:     message,
		ExitCode:    ExitValidation,
		Remediation: "Repair the reviewed memory rule provenance, evidence, lifecycle status, or review label.",
		Ref:         ref,
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

func validateSchemaContracts(root string) *CommandError {
	for _, rel := range requiredSchemaFiles {
		path := filepath.Join(root, rel)
		content, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return validationError("required schema contract files are missing", []string{rel})
			}
			return internalError("could not inspect "+rel, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(content, &schema); err != nil {
			return schemaContractError(rel+" is not valid JSON", rel)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			return schemaContractError(rel+" must declare JSON schema properties", rel)
		}
		if _, ok := properties["schema_version"]; !ok {
			return schemaContractError(rel+" must declare schema_version", rel)
		}
		if _, ok := properties["metadata"]; !ok {
			return schemaContractError(rel+" must declare forward-compatible metadata", rel)
		}
	}
	return nil
}

func loadAndValidateConfig(root string) (configSummary, []Finding, *CommandError) {
	summary := configSummary{
		GeneratedArtifactsRoot: ".relia",
		MemoryArtifactsRoot:    "memory",
		DefaultShareScope:      "private",
		SchemaVersion:          commandSchemaVersion,
		DistillEmbeddings:      "signature",
		RedactionEntropyScan:   true,
		CommitExperiences:      false,
	}
	content, err := os.ReadFile(filepath.Join(root, defaultConfigFile))
	if err != nil {
		return summary, nil, internalError("could not read relia.yaml", err)
	}

	parseConfigYAML(content, &summary)
	var warnings []Finding
	if summary.DefaultShareScope != "private" {
		return summary, warnings, memoryArtifactError("default_share_scope must remain private in the MVP", defaultConfigFile)
	}
	if summary.DistillProvider != "" && !isAllowedDistillProvider(summary.DistillProvider) {
		return summary, warnings, configError("unknown distill provider " + summary.DistillProvider)
	}
	if !isAllowedEmbeddingMode(summary.DistillEmbeddings) {
		return summary, warnings, configError("unknown distill embeddings mode " + summary.DistillEmbeddings)
	}
	if summary.DistillEmbeddings == "local" {
		if commandErr := validateLocalModelArtifact(root); commandErr != nil {
			return summary, warnings, commandErr
		}
	}
	if summary.DistillEmbeddings == "provider" && summary.DistillProvider == "" {
		return summary, warnings, configError("embeddings: provider requires distill.provider")
	}
	if summary.DistillProvider != "" {
		warnings = append(warnings, Finding{
			Type:    "live_provider_requires_explicit_grant",
			Message: "provider-backed distill is configured and requires model_provider_endpoint approval before live use",
			Ref:     "docs/dev/dev_guides.md#model-provider-and-artifact-policy",
		})
	}
	if !summary.RedactionEntropyScan {
		warnings = append(warnings, Finding{
			Type:    "redaction_entropy_scan_disabled",
			Message: "entropy scanning is disabled; generated artifacts remain internal until redaction posture is reviewed",
			Ref:     defaultConfigFile,
		})
	}
	return summary, warnings, nil
}

func parseConfigYAML(content []byte, summary *configSummary) {
	section := ""
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := stripYAMLComment(rawLine)
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		trimmed := strings.TrimSpace(line)
		if indent == 0 && strings.HasSuffix(trimmed, ":") {
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if indent != 2 || !strings.Contains(trimmed, ":") {
			continue
		}
		key, value, ok := splitYAMLKeyValue(trimmed)
		if !ok {
			continue
		}
		switch section + "." + key {
		case "artifacts.generated_root":
			if value != "" {
				summary.GeneratedArtifactsRoot = value
			}
		case "artifacts.memory_root":
			if value != "" {
				summary.MemoryArtifactsRoot = value
			}
		case "artifacts.default_share_scope":
			if value != "" {
				summary.DefaultShareScope = value
			}
		case "artifacts.schema_version":
			if value != "" {
				summary.SchemaVersion = value
			}
		case "distill.provider":
			summary.DistillProvider = value
		case "distill.embeddings":
			if value != "" {
				summary.DistillEmbeddings = value
			}
		case "redaction.entropy_scan":
			if parsed, ok := parseYAMLBool(value); ok {
				summary.RedactionEntropyScan = parsed
			}
		case "memory.commit_experiences":
			if parsed, ok := parseYAMLBool(value); ok {
				summary.CommitExperiences = parsed
			}
		}
	}
}

func stripYAMLComment(line string) string {
	if index := strings.Index(line, "#"); index >= 0 {
		return line[:index]
	}
	return line
}

func splitYAMLKeyValue(line string) (string, string, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := strings.TrimSpace(parts[0])
	value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
	return key, value, key != ""
}

func parseYAMLBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func isAllowedDistillProvider(provider string) bool {
	switch provider {
	case "anthropic", "openai-compatible":
		return true
	default:
		return false
	}
}

func isAllowedEmbeddingMode(mode string) bool {
	switch mode {
	case "signature", "local", "provider":
		return true
	default:
		return false
	}
}

func validateLocalModelArtifact(root string) *CommandError {
	manifestRel := filepath.Join(".relia", "models", "manifest.json")
	content, err := os.ReadFile(filepath.Join(root, manifestRel))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return dependencyError("local embedding artifact manifest is missing", manifestRel)
		}
		return internalError("could not inspect local model artifact manifest", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(content, &manifest); err != nil {
		return dependencyError("local embedding artifact manifest is not valid JSON", manifestRel)
	}
	required := []string{"model_id", "version", "source_url", "license", "sha256", "cache_path", "update_policy", "rollback_policy"}
	var missing []string
	for _, key := range required {
		value, ok := manifest[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return dependencyError("local embedding artifact manifest missing fields: "+strings.Join(missing, ", "), manifestRel)
	}
	return nil
}

func validateMemoryRules(root string) *CommandError {
	rulesDir := filepath.Join(root, "memory", "rules")
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return internalError("could not inspect memory rules", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		rel := filepath.Join("memory", "rules", name)
		content, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return internalError("could not read "+rel, err)
		}
		text := string(content)
		if !hasYAMLKey(text, "provenance") || !hasYAMLKey(text, "evidence") {
			return memoryArtifactError("memory rule must include provenance and evidence citations", rel)
		}
		if yamlScalarValue(text, "status") == "active" && yamlScalarValue(text, "review_label") == "" {
			return memoryArtifactError("active memory rule must include a review_label", rel)
		}
	}
	return nil
}

func hasYAMLKey(content string, key string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(stripYAMLComment(line))
		if strings.HasPrefix(trimmed, key+":") {
			return true
		}
	}
	return false
}

func yamlScalarValue(content string, key string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(stripYAMLComment(line))
		if strings.HasPrefix(trimmed, key+":") {
			_, value, ok := splitYAMLKeyValue(trimmed)
			if ok {
				return value
			}
		}
	}
	return ""
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
  revert_detection: true
  review_corrections:
    marker: "relia:correction"
  lookback_days: 180
  fix_held:
    settle_days: 14
    min_overlapping_merges: 3

redaction:
  patterns:
    - api_key
    - token
    - password
    - secret
  entropy_scan: true

distill:
  embeddings: signature
  min_evidence_count: 2
  review_required: true

memory:
  decay_half_life_days: 90
  invalidate_on_path_delete: true
  max_active_rules: 200
  commit_experiences: false

artifacts:
  schema_version: "1.0"
  generated_root: .relia
  memory_root: memory
  default_share_scope: private

serve:
  advisory_only: true

gate:
  enabled: false
`
}
