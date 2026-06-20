package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	commandResultObjectType     = "relia.command_result"
	commandResultSchemaVersion  = "1.0"
	reliaVersion                = "0.0.0-dev"
	commandModelEvidenceRef     = "docs/product/prd.md#command-model"
	jsonEnvelopeEvidenceRef     = "docs/product/prd.md#json-envelope"
	agentNativeCLIEvidenceRef   = "docs/dev/dev_guides.md#agent-native-cli-policy"
	commandResultSchemaArtifact = "schemas/command-result.schema.json"
	commandResultExamples       = "examples/command-results/"
)

type exitCode int

const (
	exitSuccess exitCode = iota
	exitGeneral
	exitInvalidUsage
	exitOutcomeObservability
	exitMemoryValidation
	exitRecurrenceGate
	exitRedactionSafety
	exitCredentialAuth
	exitDependencyNetwork
	exitProvenanceIntegrity
)

type commandResult struct {
	ObjectType      string            `json:"object_type"`
	SchemaVersion   string            `json:"schema_version"`
	Command         string            `json:"command"`
	Status          string            `json:"status"`
	Mode            string            `json:"mode"`
	ExitCode        int               `json:"exit_code"`
	Warnings        []diagnostic      `json:"warnings"`
	Errors          []diagnostic      `json:"errors"`
	Artifacts       []artifactRef     `json:"artifacts"`
	EvidenceRefs    []string          `json:"evidence_refs"`
	DurationMS      int64             `json:"duration_ms"`
	RedactionStatus string            `json:"redaction_status"`
	Metadata        map[string]string `json:"metadata"`
}

type diagnostic struct {
	Type         string            `json:"type"`
	Message      string            `json:"message"`
	ExitCode     int               `json:"exit_code"`
	EvidenceRefs []string          `json:"evidence_refs"`
	Metadata     map[string]string `json:"metadata"`
}

type artifactRef struct {
	ArtifactType string `json:"artifact_type"`
	Path         string `json:"path"`
	Description  string `json:"description"`
}

type cliOptions struct {
	JSON    bool
	Quiet   bool
	Compact bool
	Help    bool
}

var mvpCommandStubs = map[string]struct{}{
	"init":        {},
	"check":       {},
	"ingest":      {},
	"backtest":    {},
	"distill":     {},
	"review":      {},
	"memory":      {},
	"compile":     {},
	"serve":       {},
	"assess":      {},
	"demo":        {},
	"share":       {},
	"models pull": {},
}

func run(args []string, stdout io.Writer, stderr io.Writer, stdoutIsTerminal bool) int {
	start := time.Now()
	options, operands := parseGlobalOptions(args)
	result := buildCommandResult(options, operands)
	result.DurationMS = time.Since(start).Milliseconds()

	if err := writeCommandResult(stdout, stderr, result, options, stdoutIsTerminal); err != nil {
		_, _ = fmt.Fprintf(stderr, "relia: failed to write command result: %v\n", err)
		return int(exitGeneral)
	}
	return result.ExitCode
}

func parseGlobalOptions(args []string) (cliOptions, []string) {
	var options cliOptions
	operands := make([]string, 0, len(args))

	for _, arg := range args {
		switch arg {
		case "--json", "-j":
			options.JSON = true
		case "--quiet", "-q":
			options.Quiet = true
		case "--compact":
			options.Compact = true
		case "--help", "-h":
			options.Help = true
		case "--version", "-v":
			operands = append(operands, "version")
		default:
			operands = append(operands, arg)
		}
	}

	return options, operands
}

func buildCommandResult(options cliOptions, operands []string) commandResult {
	if options.Help {
		return helpResult()
	}
	if len(operands) == 0 {
		return statusResult()
	}
	if strings.HasPrefix(operands[0], "-") {
		return invalidUsageResult("status", fmt.Sprintf("unknown option %q", operands[0]))
	}

	command, rest := normalizeCommand(operands)
	switch command {
	case "status":
		if len(rest) != 0 {
			return invalidUsageResult(command, fmt.Sprintf("status does not accept arguments: %s", strings.Join(rest, " ")))
		}
		return statusResult()
	case "version":
		if len(rest) != 0 {
			return invalidUsageResult(command, fmt.Sprintf("version does not accept arguments: %s", strings.Join(rest, " ")))
		}
		return versionResult()
	case "help":
		return helpResult()
	case "models":
		return invalidUsageResult(command, `models requires the "pull" subcommand`)
	}

	if _, ok := mvpCommandStubs[command]; ok {
		return notImplementedResult(command)
	}

	return invalidUsageResult(command, fmt.Sprintf("unknown command %q", command))
}

func normalizeCommand(operands []string) (string, []string) {
	if len(operands) >= 2 && operands[0] == "models" && operands[1] == "pull" {
		return "models pull", operands[2:]
	}
	return operands[0], operands[1:]
}

func statusResult() commandResult {
	result := baseResult("status", "status", exitSuccess)
	result.Artifacts = []artifactRef{
		{
			ArtifactType: "schema",
			Path:         commandResultSchemaArtifact,
			Description:  "CommandResult JSON schema.",
		},
		{
			ArtifactType: "examples",
			Path:         commandResultExamples,
			Description:  "Stable exit-code command-result examples.",
		},
	}
	result.Metadata["version"] = reliaVersion
	return result
}

func versionResult() commandResult {
	result := baseResult("version", "version", exitSuccess)
	result.Metadata["version"] = reliaVersion
	return result
}

func helpResult() commandResult {
	result := baseResult("help", "help", exitSuccess)
	result.Metadata["usage"] = "relia [--json] [--quiet] [--compact] <command>"
	result.Metadata["commands"] = "status, version, help, init, check, ingest, backtest, distill, review, memory, compile, serve, assess, models pull, demo, share"
	return result
}

func notImplementedResult(command string) commandResult {
	result := baseResult(command, modeForCommand(command), exitGeneral)
	result.Status = "error"
	result.Errors = []diagnostic{
		{
			Type:     "not_implemented",
			Message:  fmt.Sprintf("relia %s is planned for the MVP but is not implemented in this foundation slice", command),
			ExitCode: int(exitGeneral),
			EvidenceRefs: []string{
				commandModelEvidenceRef,
				agentNativeCLIEvidenceRef,
			},
			Metadata: map[string]string{
				"stability": "typed_command_stub",
			},
		},
	}
	result.EvidenceRefs = []string{
		commandModelEvidenceRef,
		agentNativeCLIEvidenceRef,
	}
	return result
}

func invalidUsageResult(command string, message string) commandResult {
	if command == "" {
		command = "status"
	}
	result := baseResult(command, modeForCommand(command), exitInvalidUsage)
	result.Status = "error"
	result.Errors = []diagnostic{
		{
			Type:     "invalid_usage",
			Message:  message,
			ExitCode: int(exitInvalidUsage),
			EvidenceRefs: []string{
				commandModelEvidenceRef,
			},
			Metadata: map[string]string{
				"exit_meaning": "invalid usage, invalid input, parse error, or local configuration error",
			},
		},
	}
	result.EvidenceRefs = []string{commandModelEvidenceRef}
	return result
}

func baseResult(command string, mode string, code exitCode) commandResult {
	return commandResult{
		ObjectType:    commandResultObjectType,
		SchemaVersion: commandResultSchemaVersion,
		Command:       command,
		Status:        "pass",
		Mode:          mode,
		ExitCode:      int(code),
		Warnings:      []diagnostic{},
		Errors:        []diagnostic{},
		Artifacts:     []artifactRef{},
		EvidenceRefs: []string{
			jsonEnvelopeEvidenceRef,
			agentNativeCLIEvidenceRef,
		},
		DurationMS:      0,
		RedactionStatus: "not_applicable",
		Metadata:        map[string]string{},
	}
}

func modeForCommand(command string) string {
	replacer := strings.NewReplacer(" ", "_", "-", "_")
	return replacer.Replace(command)
}

func writeCommandResult(stdout io.Writer, stderr io.Writer, result commandResult, options cliOptions, stdoutIsTerminal bool) error {
	if shouldWriteJSON(options, stdoutIsTerminal) {
		encoder := json.NewEncoder(stdout)
		if !options.Compact && !options.Quiet {
			encoder.SetIndent("", "  ")
		}
		return encoder.Encode(result)
	}
	return writeHumanResult(stdout, stderr, result)
}

func shouldWriteJSON(options cliOptions, stdoutIsTerminal bool) bool {
	return options.JSON || options.Quiet || options.Compact || !stdoutIsTerminal
}

func writeHumanResult(stdout io.Writer, stderr io.Writer, result commandResult) error {
	if result.Status == "pass" {
		switch result.Command {
		case "help":
			_, err := fmt.Fprint(stdout, usageText())
			return err
		case "version":
			_, err := fmt.Fprintf(stdout, "relia %s\n", reliaVersion)
			return err
		default:
			_, err := fmt.Fprintf(stdout, "relia %s: %s\n", result.Command, result.Status)
			return err
		}
	}

	message := fmt.Sprintf("relia %s: %s", result.Command, result.Status)
	if len(result.Errors) > 0 {
		message = fmt.Sprintf("relia %s: %s", result.Command, result.Errors[0].Message)
	}
	_, err := fmt.Fprintf(stderr, "%s (exit %d)\n", message, result.ExitCode)
	return err
}

func usageText() string {
	return strings.Join([]string{
		"Relia CLI",
		"",
		"Usage:",
		"  relia [--json] [--quiet] [--compact] <command>",
		"",
		"Commands:",
		"  status, version, help",
		"  init, check, ingest, backtest, distill, review, memory, compile, serve, assess",
		"  models pull, demo, share",
		"",
	}, "\n")
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, isTerminal(os.Stdout)))
}
