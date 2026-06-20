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
	"schemas/command-result.schema.json",
	"schemas/relia-config.schema.json",
	"schemas/experience-record.schema.json",
	"schemas/memory-rule.schema.json",
	"examples/command-results/exit-code-examples.json",
	"memory/README.md",
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
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type configContract struct {
	Name          string
	Path          string
	Schema        string
	SchemaVersion string
}

type configValidation struct {
	SchemaVersion           string
	ArtifactContractVersion string
	PrivacyDefaults         map[string]any
	ArtifactContracts       []configContract
	SchemaVersions          map[string]string
}

type yamlDocument struct {
	scalars map[string]string
	lists   map[string][]string
}

type expectedScalar struct {
	path string
	want string
}

var reliaArtifactContracts = []configContract{
	{
		Name:          "command_results",
		Schema:        "schemas/command-result.schema.json",
		SchemaVersion: commandSchemaVersion,
	},
	{
		Name:          "config",
		Path:          defaultConfigFile,
		Schema:        "schemas/relia-config.schema.json",
		SchemaVersion: commandSchemaVersion,
	},
	{
		Name:          "experiences",
		Path:          "memory/experiences.jsonl",
		Schema:        "schemas/experience-record.schema.json",
		SchemaVersion: commandSchemaVersion,
	},
	{
		Name:          "memory_rules",
		Path:          "memory/rules.jsonl",
		Schema:        "schemas/memory-rule.schema.json",
		SchemaVersion: commandSchemaVersion,
	},
}

var checkArtifactRefs = []ArtifactRef{
	{Kind: "config", Path: defaultConfigFile},
	{Kind: "schema", Path: "schemas/command-result.schema.json"},
	{Kind: "schema", Path: "schemas/relia-config.schema.json"},
	{Kind: "schema", Path: "schemas/experience-record.schema.json"},
	{Kind: "schema", Path: "schemas/memory-rule.schema.json"},
	{Kind: "artifact_contract", Path: "memory/"},
	{Kind: "example", Path: "examples/command-results/exit-code-examples.json"},
}

var requiredPrivacyFields = []string{
	"owner",
	"credential",
	"secret",
	"endpoint",
	"machine_local_path",
}

var requiredConfigScalars = []expectedScalar{
	{path: "version", want: "1"},
	{path: "schema_version", want: commandSchemaVersion},
	{path: "artifact_contract_version", want: commandSchemaVersion},
	{path: "repo.provider", want: "github"},
	{path: "attribution.uncertain", want: "exclude"},
	{path: "privacy.default_scope", want: "local_only"},
	{path: "privacy.customer_safe_default", want: "false"},
	{path: "privacy.org_eligible_default", want: "false"},
	{path: "privacy.redaction.capture_time", want: "true"},
	{path: "privacy.redaction.recursive", want: "true"},
	{path: "network.default", want: "disabled"},
	{path: "network.credentials", want: "none"},
	{path: "distill.embeddings", want: "signature"},
	{path: "distill.local_artifacts.required_pull", want: "true"},
	{path: "distill.provider.enabled", want: "false"},
	{path: "serve.advisory_only", want: "true"},
	{path: "gate.enabled", want: "false"},
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

	configValidation, err := validateReliaConfig(root)
	if err != nil {
		result := errorResult("check", "check", configurationValidationError(err.Error()), start)
		result.RedactionStatus = "failed_closed"
		return result
	}

	result := passResult("check", "check", "local operating pack baseline is present", start, map[string]any{
		"artifact_contract_version": configValidation.ArtifactContractVersion,
		"artifact_contracts":        artifactContractData(configValidation.ArtifactContracts),
		"checked_paths":             len(requiredCheckFiles),
		"config_schema_version":     configValidation.SchemaVersion,
		"privacy_defaults":          configValidation.PrivacyDefaults,
		"repo_root":                 ".",
		"schema_versions":           configValidation.SchemaVersions,
	})
	result.Artifacts = append(result.Artifacts, checkArtifactRefs...)
	return result
}

func validateReliaConfig(root string) (configValidation, error) {
	content, err := os.ReadFile(filepath.Join(root, defaultConfigFile))
	if err != nil {
		return configValidation{}, fmt.Errorf("could not read %s: %w", defaultConfigFile, err)
	}
	document, err := parseReliaYAML(content)
	if err != nil {
		return configValidation{}, fmt.Errorf("%s is invalid: %w", defaultConfigFile, err)
	}

	for _, expected := range requiredConfigScalars {
		if err := requireScalar(document, expected.path, expected.want); err != nil {
			return configValidation{}, err
		}
	}
	if err := requireListContains(document, "privacy.redaction.nested_fields", requiredPrivacyFields); err != nil {
		return configValidation{}, err
	}

	schemaVersions := map[string]string{}
	for _, contract := range reliaArtifactContracts {
		prefix := "artifact_contracts." + contract.Name
		if contract.Path != "" {
			if err := requireScalar(document, prefix+".path", contract.Path); err != nil {
				return configValidation{}, err
			}
			if err := validateRepoRelativePath(contract.Path); err != nil {
				return configValidation{}, fmt.Errorf("%s.path is invalid: %w", prefix, err)
			}
			if err := validateArtifactParent(root, contract.Path); err != nil {
				return configValidation{}, fmt.Errorf("%s.path is invalid: %w", prefix, err)
			}
		}
		if err := requireScalar(document, prefix+".schema", contract.Schema); err != nil {
			return configValidation{}, err
		}
		if err := validateRepoRelativePath(contract.Schema); err != nil {
			return configValidation{}, fmt.Errorf("%s.schema is invalid: %w", prefix, err)
		}
		if err := requireScalar(document, prefix+".schema_version", contract.SchemaVersion); err != nil {
			return configValidation{}, err
		}
		schemaVersion, err := validateSchemaFile(root, contract.Schema)
		if err != nil {
			return configValidation{}, fmt.Errorf("%s.schema is invalid: %w", prefix, err)
		}
		if schemaVersion != contract.SchemaVersion {
			return configValidation{}, fmt.Errorf("%s.schema_version must match %s schema version %s", prefix, contract.Schema, schemaVersion)
		}
		schemaVersions[contract.Schema] = schemaVersion
	}

	if err := validateJSONFile(root, "examples/command-results/exit-code-examples.json"); err != nil {
		return configValidation{}, err
	}

	return configValidation{
		SchemaVersion:           commandSchemaVersion,
		ArtifactContractVersion: commandSchemaVersion,
		ArtifactContracts:       append([]configContract(nil), reliaArtifactContracts...),
		SchemaVersions:          schemaVersions,
		PrivacyDefaults: map[string]any{
			"default_scope":          "local_only",
			"customer_safe_default":  false,
			"org_eligible_default":   false,
			"redaction_capture_time": true,
			"redaction_recursive":    true,
		},
	}, nil
}

func parseReliaYAML(content []byte) (yamlDocument, error) {
	document := yamlDocument{
		scalars: map[string]string{},
		lists:   map[string][]string{},
	}
	stack := []string{}
	lines := strings.Split(string(content), "\n")
	for lineNumber, raw := range lines {
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		if strings.Contains(raw, "\t") {
			return yamlDocument{}, fmt.Errorf("line %d uses tabs; use two-space indentation", lineNumber+1)
		}
		indent := leadingSpaces(raw)
		if indent%2 != 0 {
			return yamlDocument{}, fmt.Errorf("line %d uses odd indentation", lineNumber+1)
		}
		level := indent / 2
		if level > len(stack) {
			return yamlDocument{}, fmt.Errorf("line %d skips an indentation level", lineNumber+1)
		}
		stack = stack[:level]
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "- ") {
			if len(stack) == 0 {
				return yamlDocument{}, fmt.Errorf("line %d has a list item without a parent key", lineNumber+1)
			}
			path := strings.Join(stack, ".")
			document.lists[path] = append(document.lists[path], normalizeYAMLScalar(strings.TrimPrefix(trimmed, "- ")))
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return yamlDocument{}, fmt.Errorf("line %d is not a key-value entry", lineNumber+1)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return yamlDocument{}, fmt.Errorf("line %d has an empty key", lineNumber+1)
		}
		pathParts := append(append([]string{}, stack...), key)
		path := strings.Join(pathParts, ".")
		value = strings.TrimSpace(value)
		if value == "" {
			stack = append(stack, key)
			continue
		}
		document.scalars[path] = normalizeYAMLScalar(value)
	}
	return document, nil
}

func leadingSpaces(value string) int {
	count := 0
	for _, char := range value {
		if char != ' ' {
			break
		}
		count++
	}
	return count
}

func normalizeYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func requireScalar(document yamlDocument, path string, want string) error {
	got, ok := document.scalars[path]
	if !ok {
		return fmt.Errorf("%s must be %s", path, want)
	}
	if got != want {
		return fmt.Errorf("%s must be %s", path, want)
	}
	return nil
}

func requireListContains(document yamlDocument, path string, want []string) error {
	values := map[string]bool{}
	for _, value := range document.lists[path] {
		values[value] = true
	}
	for _, value := range want {
		if !values[value] {
			return fmt.Errorf("%s must include %s", path, value)
		}
	}
	return nil
}

func validateRepoRelativePath(rel string) error {
	if rel == "" {
		return errors.New("path is empty")
	}
	if filepath.IsAbs(rel) {
		return errors.New("absolute paths are not allowed")
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return errors.New("path must stay inside the repository")
	}
	return nil
}

func validateArtifactParent(root string, rel string) error {
	parent := filepath.Dir(filepath.Join(root, rel))
	if _, err := os.Stat(parent); err != nil {
		return fmt.Errorf("parent directory %s is missing", filepath.ToSlash(filepath.Dir(rel)))
	}
	return nil
}

func validateSchemaFile(root string, rel string) (string, error) {
	content, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return "", err
	}
	var schema map[string]any
	if err := json.Unmarshal(content, &schema); err != nil {
		return "", err
	}
	for _, key := range []string{"$schema", "$id", "title"} {
		value, ok := schema[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("missing string %s", key)
		}
	}
	if schema["type"] != "object" {
		return "", errors.New("schema root type must be object")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return "", errors.New("schema must define properties")
	}
	schemaVersion, ok := properties["schema_version"].(map[string]any)
	if !ok {
		return "", errors.New("schema must define schema_version")
	}
	version, ok := schemaVersion["const"].(string)
	if !ok || strings.TrimSpace(version) == "" {
		return "", errors.New("schema_version must define a const version")
	}
	return version, nil
}

func validateJSONFile(root string, rel string) error {
	content, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return fmt.Errorf("could not read %s: %w", rel, err)
	}
	var payload any
	if err := json.Unmarshal(content, &payload); err != nil {
		return fmt.Errorf("%s is not valid JSON: %w", rel, err)
	}
	return nil
}

func artifactContractData(contracts []configContract) []map[string]string {
	data := make([]map[string]string, 0, len(contracts))
	for _, contract := range contracts {
		data = append(data, map[string]string{
			"name":           contract.Name,
			"path":           contract.Path,
			"schema":         contract.Schema,
			"schema_version": contract.SchemaVersion,
		})
	}
	return data
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

func validationError(message string, missing []string) *CommandError {
	return &CommandError{
		Type:        "operating_pack_validation_failed",
		Message:     message + ": " + strings.Join(missing, ", "),
		ExitCode:    ExitValidation,
		Remediation: "Restore the required repo lifecycle files before running Relia workflows.",
		Ref:         "docs/dev/dev_guides.md#validation-matrix",
	}
}

func configurationValidationError(message string) *CommandError {
	return &CommandError{
		Type:        "configuration_validation_failed",
		Message:     message,
		ExitCode:    ExitValidation,
		Remediation: "Restore relia.yaml to the versioned offline-safe defaults from relia init.",
		Ref:         defaultConfigFile,
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
artifact_contract_version: "1.0"

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
  default_scope: local_only
  customer_safe_default: false
  org_eligible_default: false
  redaction:
    capture_time: true
    recursive: true
    nested_fields:
      - owner
      - credential
      - secret
      - endpoint
      - machine_local_path

network:
  default: disabled
  credentials: none

artifact_contracts:
  command_results:
    schema: schemas/command-result.schema.json
    schema_version: "1.0"
  config:
    path: relia.yaml
    schema: schemas/relia-config.schema.json
    schema_version: "1.0"
  experiences:
    path: memory/experiences.jsonl
    schema: schemas/experience-record.schema.json
    schema_version: "1.0"
  memory_rules:
    path: memory/rules.jsonl
    schema: schemas/memory-rule.schema.json
    schema_version: "1.0"

distill:
  embeddings: signature
  local_artifacts:
    required_pull: true
  provider:
    enabled: false

serve:
  advisory_only: true

gate:
  enabled: false
`
}
