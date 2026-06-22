package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
			return errorResult("init", "init", internalError("could not write artifact skeleton", err), start)
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

func ensureArtifactSkeleton(root string) error {
	for _, dir := range artifactSkeletonDirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return err
		}
	}
	return nil
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
		"gate.enabled":                    "false",
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
	var warnings []Finding
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

	if gateLimit, ok := document.Scalars["gate.max_error_recurrence_rate"]; ok {
		warnings = append(warnings, Finding{
			Type:    "unenforced_gate_setting",
			Message: "gate.max_error_recurrence_rate is configured while gate.enabled is false",
			Ref:     configRef(gateLimit),
		})
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
	if filepath.IsAbs(manifest.CachePath) {
		return dependencyError("local model manifest cache_path must be repo-relative", manifestRel)
	}
	artifactPath := filepath.Join(root, manifest.CachePath)
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
