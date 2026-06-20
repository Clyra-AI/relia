package main

import (
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
	defaultConfigFile       = "relia.yaml"
)

var requiredPhase0SchemaFiles = []string{
	"schemas/command-result.schema.json",
	"schemas/compiled-context.schema.json",
	"schemas/coverage-map.schema.json",
	"schemas/experience-record.schema.json",
	"schemas/failure-signature.schema.json",
	"schemas/memory-rule.schema.json",
	"schemas/outcome-evidence.schema.json",
	"schemas/recurrence-report.schema.json",
	"schemas/redaction-config.schema.json",
	"schemas/risk-assessment.schema.json",
}

var requiredArtifactDirs = []string{
	"memory/rules",
	"memory/compiled",
	"examples/command-results",
	"examples/reports",
}

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

type schemaContract struct {
	Schema     string         `json:"$schema"`
	ID         string         `json:"$id"`
	Type       string         `json:"type"`
	Required   []string       `json:"required"`
	Properties map[string]any `json:"properties"`
}

type yamlKind int

const (
	yamlUnset yamlKind = iota
	yamlMap
	yamlList
	yamlScalar
)

type yamlNode struct {
	kind     yamlKind
	scalar   any
	children map[string]*yamlNode
	items    []*yamlNode
}

type yamlFrame struct {
	indent int
	node   *yamlNode
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
			return errorResult("init", "init", internalError("could not write artifact skeleton", err), start)
		}
		result := passResult("init", "init", "relia.yaml already exists", start, map[string]any{
			"config_path": defaultConfigFile,
			"created":     false,
		})
		result.Artifacts = append(result.Artifacts, artifact)
		result.Artifacts = append(result.Artifacts, artifactSkeletonRefs()...)
		return result
	} else if !errors.Is(err, os.ErrNotExist) {
		return errorResult("init", "init", internalError("could not inspect relia.yaml", err), start)
	}

	if err := ensureArtifactSkeleton(root); err != nil {
		return errorResult("init", "init", internalError("could not write artifact skeleton", err), start)
	}
	if err := os.WriteFile(configPath, []byte(defaultConfigYAML()), 0o644); err != nil {
		return errorResult("init", "init", internalError("could not write relia.yaml", err), start)
	}
	result := passResult("init", "init", "created relia.yaml", start, map[string]any{
		"config_path": defaultConfigFile,
		"created":     true,
	})
	result.Artifacts = append(result.Artifacts, artifact)
	result.Artifacts = append(result.Artifacts, artifactSkeletonRefs()...)
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
	for _, rel := range requiredPhase0SchemaFiles {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				missing = append(missing, rel)
				continue
			}
			return errorResult("check", "check", internalError("could not inspect "+rel, err), start)
		}
	}
	for _, rel := range requiredArtifactDirs {
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				missing = append(missing, rel+"/")
				continue
			}
			return errorResult("check", "check", internalError("could not inspect "+rel, err), start)
		}
		if !info.IsDir() {
			missing = append(missing, rel+"/")
		}
	}
	if len(missing) > 0 {
		return errorResult("check", "check", validationError("required local operating-pack files are missing", missing), start)
	}
	configIssues, redactionIssues, err := validateConfigFile(filepath.Join(root, defaultConfigFile))
	if err != nil {
		return errorResult("check", "check", internalError("could not inspect relia.yaml", err), start)
	}
	if len(redactionIssues) > 0 {
		return errorResult("check", "check", redactionSafetyError("relia.yaml redaction defaults are unsafe or incomplete: "+strings.Join(redactionIssues, ", ")), start)
	}
	if len(configIssues) > 0 {
		return errorResult("check", "check", configError("relia.yaml is missing required PRD defaults: "+strings.Join(configIssues, ", ")), start)
	}
	if issues, err := validateSchemaContracts(root); err != nil {
		return errorResult("check", "check", internalError("could not inspect schema contracts", err), start)
	} else if len(issues) > 0 {
		return errorResult("check", "check", schemaValidationError("schema contracts are incomplete: "+strings.Join(issues, ", ")), start)
	}

	return passResult("check", "check", "local operating pack baseline is present", start, map[string]any{
		"artifact_dirs":  requiredArtifactDirs,
		"checked_paths":  len(requiredCheckFiles) + len(requiredPhase0SchemaFiles) + len(requiredArtifactDirs),
		"config_path":    defaultConfigFile,
		"repo_root":      ".",
		"schema_count":   len(requiredPhase0SchemaFiles),
		"schema_version": commandSchemaVersion,
		"privacy_defaults": map[string]any{
			"commit_experiences": false,
			"embeddings":         "signature",
			"entropy_scan":       true,
			"gate_enabled":       false,
			"share_scope":        "private",
		},
	})
}

func ensureArtifactSkeleton(root string) error {
	for _, rel := range requiredArtifactDirs {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func artifactSkeletonRefs() []ArtifactRef {
	refs := make([]ArtifactRef, 0, len(requiredArtifactDirs))
	for _, rel := range requiredArtifactDirs {
		refs = append(refs, ArtifactRef{Kind: "artifact_dir", Path: rel})
	}
	return refs
}

func validateConfigFile(path string) ([]string, []string, error) {
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	config, err := parseYAMLSubset(contentBytes)
	if err != nil {
		return []string{"parse relia.yaml: " + err.Error()}, nil, nil
	}
	configIssues, redactionIssues := validatePhase0Config(config)
	return configIssues, redactionIssues, nil
}

func validatePhase0Config(config *yamlNode) ([]string, []string) {
	var configIssues []string
	var redactionIssues []string

	requireScalar(&configIssues, config, []string{"version"}, 1)
	requireScalar(&configIssues, config, []string{"repo", "provider"}, "github")
	requireScalar(&configIssues, config, []string{"repo", "remote"}, "origin")
	requireEmptyList(&configIssues, config, []string{"repo", "scopes"})
	requireEmptyList(&configIssues, config, []string{"attribution", "agent_authors"})
	requireScalar(&configIssues, config, []string{"attribution", "uncertain"}, "exclude")
	requireMap(&configIssues, config, []string{"outcomes", "checks"})
	requireScalar(&configIssues, config, []string{"outcomes", "revert_detection"}, true)
	requireScalar(&configIssues, config, []string{"outcomes", "lookback_days"}, 180)
	requireScalar(&configIssues, config, []string{"outcomes", "fix_held", "settle_days"}, 14)
	requireScalar(&configIssues, config, []string{"outcomes", "fix_held", "min_overlapping_merges"}, 3)
	requireScalar(&configIssues, config, []string{"distill", "embeddings"}, "signature")
	requireScalar(&configIssues, config, []string{"distill", "review_required"}, true)
	requireScalar(&configIssues, config, []string{"memory", "decay_half_life_days"}, 90)
	requireScalar(&configIssues, config, []string{"memory", "commit_experiences"}, false)
	requireScalar(&configIssues, config, []string{"serve", "mcp"}, true)
	requireScalar(&configIssues, config, []string{"advise", "enabled"}, true)
	requireScalar(&configIssues, config, []string{"advise", "max_comments_per_pr"}, 1)
	requireScalar(&configIssues, config, []string{"gate", "enabled"}, false)

	requireListContains(&redactionIssues, config, []string{"redaction", "patterns"}, []string{"api_key", "token", "password", "secret"})
	requireScalar(&redactionIssues, config, []string{"redaction", "entropy_scan"}, true)

	return configIssues, redactionIssues
}

func requireMap(issues *[]string, root *yamlNode, path []string) {
	node, ok := root.lookup(path...)
	if !ok || node.kind != yamlMap {
		*issues = append(*issues, strings.Join(path, ".")+" must be a map")
	}
}

func requireEmptyList(issues *[]string, root *yamlNode, path []string) {
	node, ok := root.lookup(path...)
	if !ok || node.kind != yamlList || len(node.items) != 0 {
		*issues = append(*issues, strings.Join(path, ".")+" must be []")
	}
}

func requireListContains(issues *[]string, root *yamlNode, path []string, expected []string) {
	node, ok := root.lookup(path...)
	if !ok || node.kind != yamlList {
		*issues = append(*issues, strings.Join(path, ".")+" must list "+strings.Join(expected, ", "))
		return
	}
	present := make(map[string]bool, len(node.items))
	for _, item := range node.items {
		if item.kind == yamlScalar {
			if value, ok := item.scalar.(string); ok {
				present[value] = true
			}
		}
	}
	for _, value := range expected {
		if !present[value] {
			*issues = append(*issues, strings.Join(path, ".")+" missing "+value)
		}
	}
}

func requireScalar(issues *[]string, root *yamlNode, path []string, expected any) {
	node, ok := root.lookup(path...)
	if !ok || node.kind != yamlScalar || !scalarEqual(node.scalar, expected) {
		*issues = append(*issues, strings.Join(path, ".")+" must be "+fmt.Sprint(expected))
	}
}

func scalarEqual(actual any, expected any) bool {
	switch want := expected.(type) {
	case int:
		got, ok := actual.(int)
		return ok && got == want
	case bool:
		got, ok := actual.(bool)
		return ok && got == want
	case string:
		got, ok := actual.(string)
		return ok && got == want
	default:
		return actual == expected
	}
}

func (node *yamlNode) lookup(path ...string) (*yamlNode, bool) {
	current := node
	for _, segment := range path {
		if current == nil || current.kind != yamlMap {
			return nil, false
		}
		next, ok := current.children[segment]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func parseYAMLSubset(content []byte) (*yamlNode, error) {
	root := &yamlNode{kind: yamlMap, children: map[string]*yamlNode{}}
	stack := []yamlFrame{{indent: -1, node: root}}

	for lineIndex, raw := range strings.Split(string(content), "\n") {
		lineNumber := lineIndex + 1
		line, err := stripYAMLComment(raw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.Contains(line[:leadingWhitespace(line)], "\t") {
			return nil, fmt.Errorf("line %d: tabs are not supported for indentation", lineNumber)
		}

		indent := countLeadingSpaces(line)
		trimmed := strings.TrimSpace(line)
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			return nil, fmt.Errorf("line %d: invalid indentation", lineNumber)
		}
		parent := stack[len(stack)-1].node

		if strings.HasPrefix(trimmed, "- ") {
			if err := parent.ensureList(); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			itemText := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			item := &yamlNode{}
			if itemText == "" {
				stack = append(stack, yamlFrame{indent: indent, node: item})
			} else {
				item.kind = yamlScalar
				item.scalar = parseYAMLScalar(itemText)
			}
			parent.items = append(parent.items, item)
			continue
		}

		key, value, ok := splitYAMLKeyValue(trimmed)
		if !ok {
			return nil, fmt.Errorf("line %d: expected key: value", lineNumber)
		}
		if err := parent.ensureMap(); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		child := &yamlNode{}
		switch {
		case value == "":
			stack = append(stack, yamlFrame{indent: indent, node: child})
		case value == "[]":
			child.kind = yamlList
		default:
			child.kind = yamlScalar
			child.scalar = parseYAMLScalar(value)
		}
		parent.children[key] = child
	}

	return root, nil
}

func (node *yamlNode) ensureMap() error {
	switch node.kind {
	case yamlUnset:
		node.kind = yamlMap
		node.children = map[string]*yamlNode{}
	case yamlMap:
	default:
		return fmt.Errorf("cannot add map key under non-map value")
	}
	return nil
}

func (node *yamlNode) ensureList() error {
	switch node.kind {
	case yamlUnset:
		node.kind = yamlList
	case yamlList:
	default:
		return fmt.Errorf("cannot add list item under non-list value")
	}
	return nil
}

func stripYAMLComment(line string) (string, error) {
	inSingle := false
	inDouble := false
	escaped := false
	for index, r := range line {
		switch {
		case escaped:
			escaped = false
		case inDouble && r == '\\':
			escaped = true
		case inDouble && r == '"':
			inDouble = !inDouble
		case inSingle && r == '\'':
			inSingle = !inSingle
		case r == '"' && !inSingle && quoteStartsScalar(line, index):
			inDouble = true
		case r == '\'' && !inDouble && quoteStartsScalar(line, index):
			inSingle = true
		case r == '#' && !inSingle && !inDouble && commentStarts(line, index):
			return strings.TrimRight(line[:index], " \t"), nil
		}
	}
	if inSingle || inDouble {
		return "", fmt.Errorf("unterminated quoted scalar")
	}
	return strings.TrimRight(line, " \t"), nil
}

func quoteStartsScalar(line string, index int) bool {
	for i := index - 1; i >= 0; i-- {
		switch line[i] {
		case ' ', '\t':
			continue
		case ':', '-':
			return true
		default:
			return false
		}
	}
	return true
}

func commentStarts(line string, index int) bool {
	if index == 0 {
		return true
	}
	return line[index-1] == ' ' || line[index-1] == '\t'
}

func splitYAMLKeyValue(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	return key, strings.TrimSpace(value), true
}

func parseYAMLScalar(value string) any {
	switch value {
	case "true":
		return true
	case "false":
		return false
	case "null", "~":
		return nil
	}
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return strings.ReplaceAll(strings.Trim(value, "'"), "''", "'")
	}
	if number, err := strconv.Atoi(value); err == nil {
		return number
	}
	return value
}

func countLeadingSpaces(line string) int {
	count := 0
	for _, r := range line {
		if r != ' ' {
			return count
		}
		count++
	}
	return count
}

func leadingWhitespace(line string) int {
	count := 0
	for _, r := range line {
		if r != ' ' && r != '\t' {
			return count
		}
		count++
	}
	return count
}

func validateSchemaContracts(root string) ([]string, error) {
	var issues []string
	for _, rel := range requiredPhase0SchemaFiles {
		schemaIssues, err := validateSchemaFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		for _, issue := range schemaIssues {
			issues = append(issues, rel+" "+issue)
		}
	}
	return issues, nil
}

func validateSchemaFile(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var contract schemaContract
	if err := json.Unmarshal(content, &contract); err != nil {
		return []string{"is not valid JSON"}, nil
	}
	var issues []string
	if contract.Schema == "" {
		issues = append(issues, "missing $schema")
	}
	if contract.ID == "" {
		issues = append(issues, "missing $id")
	}
	if contract.Type != "object" {
		issues = append(issues, "must declare type object")
	}
	required := map[string]bool{}
	for _, field := range contract.Required {
		required[field] = true
	}
	for _, field := range []string{"object_type", "schema_version"} {
		if !required[field] {
			issues = append(issues, "required missing "+field)
		}
	}
	if _, ok := contract.Properties["metadata"]; !ok {
		issues = append(issues, "missing metadata property")
	}
	schemaVersion, ok := contract.Properties["schema_version"].(map[string]any)
	if !ok {
		issues = append(issues, "missing schema_version property")
	} else if schemaVersion["const"] != commandSchemaVersion {
		issues = append(issues, "schema_version const must be "+commandSchemaVersion)
	}
	if _, ok := contract.Properties["object_type"].(map[string]any); !ok {
		issues = append(issues, "missing object_type property")
	}
	return issues, nil
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

func schemaValidationError(message string) *CommandError {
	return &CommandError{
		Type:        "artifact_schema_validation_failed",
		Message:     message,
		ExitCode:    ExitValidation,
		Remediation: "Repair the versioned schemas under schemas/ before running Relia workflows.",
		Ref:         "docs/product/prd.md#json-schema-contract",
	}
}

func redactionSafetyError(message string) *CommandError {
	return &CommandError{
		Type:        "redaction_safety_failed",
		Message:     message,
		ExitCode:    ExitRedactionSafety,
		Remediation: "Restore fail-closed redaction patterns and entropy scanning before persisting or sharing artifacts.",
		Ref:         "docs/product/prd.md#redaction-pipeline-contract",
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

serve:
  mcp: true
  advisory_only: true
  compile:
    targets:
      - AGENTS.md
      - CLAUDE.md
    max_rules: 25

advise:
  enabled: true
  max_comments_per_pr: 1
  update_in_place: true
  reassess_debounce_minutes: 10
  min_confidence: 0.6

badge:
  stale_after_days: 30
  stale_after_merged_prs: 20

gate:
  enabled: false
  max_error_recurrence_rate: null
`
}
