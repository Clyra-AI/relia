package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func runCommand(t *testing.T, args ...string) (int, commandResult, string, string) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(args, &stdout, &stderr, false)

	var result commandResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout was not a command result JSON object: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	return code, result, stdout.String(), stderr.String()
}

func TestRunDefaultsToMachineReadableStatusWhenNonInteractive(t *testing.T) {
	code, result, _, stderr := runCommand(t)

	if code != int(exitSuccess) {
		t.Fatalf("exit code = %d, want %d", code, exitSuccess)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if result.ObjectType != "relia.command_result" {
		t.Fatalf("object_type = %q", result.ObjectType)
	}
	if result.SchemaVersion != commandResultSchemaVersion {
		t.Fatalf("schema_version = %q", result.SchemaVersion)
	}
	if result.Command != "status" || result.Status != "pass" || result.Mode != "status" {
		t.Fatalf("unexpected command result: %#v", result)
	}
	if result.ExitCode != int(exitSuccess) {
		t.Fatalf("exit_code = %d, want %d", result.ExitCode, exitSuccess)
	}
	if result.RedactionStatus != "not_applicable" {
		t.Fatalf("redaction_status = %q", result.RedactionStatus)
	}
	if result.EvidenceRefs == nil {
		t.Fatalf("evidence_refs must be present even when empty")
	}
	if result.Errors == nil {
		t.Fatalf("errors must be present even when empty")
	}
}

func TestRunSupportsExplicitJSONModeOnTerminal(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--json", "version"}, &stdout, &stderr, true)

	if code != int(exitSuccess) {
		t.Fatalf("exit code = %d, want %d", code, exitSuccess)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	var result commandResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("--json stdout was not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if result.Command != "version" || result.Status != "pass" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Metadata["version"] == "" {
		t.Fatalf("version metadata must be populated")
	}
}

func TestHelpFlagProducesMachineReadableHelpWhenNonInteractive(t *testing.T) {
	code, result, _, stderr := runCommand(t, "--help")

	if code != int(exitSuccess) {
		t.Fatalf("exit code = %d, want %d", code, exitSuccess)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if result.Command != "help" || result.Mode != "help" {
		t.Fatalf("unexpected help result: %#v", result)
	}
	if !strings.Contains(result.Metadata["commands"], "models pull") {
		t.Fatalf("help metadata should list commands, got %#v", result.Metadata)
	}
}

func TestRunUsesHumanOutputOnlyForInteractiveDefault(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"status"}, &stdout, &stderr, true)

	if code != int(exitSuccess) {
		t.Fatalf("exit code = %d, want %d", code, exitSuccess)
	}
	if got, want := stdout.String(), "relia status: pass\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestVersionFlagUsesHumanOutputOnTerminal(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--version"}, &stdout, &stderr, true)

	if code != int(exitSuccess) {
		t.Fatalf("exit code = %d, want %d", code, exitSuccess)
	}
	if got, want := stdout.String(), "relia 0.0.0-dev\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestHelpUsesHumanOutputOnTerminal(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"help"}, &stdout, &stderr, true)

	if code != int(exitSuccess) {
		t.Fatalf("exit code = %d, want %d", code, exitSuccess)
	}
	if !strings.Contains(stdout.String(), "Usage:") || !strings.Contains(stdout.String(), "models pull") {
		t.Fatalf("stdout should contain usage and command list, got %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestQuietAndCompactPreserveMachineReadableFields(t *testing.T) {
	for _, flag := range []string{"--quiet", "--compact"} {
		t.Run(flag, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			code := run([]string{flag, "status"}, &stdout, &stderr, true)

			if code != int(exitSuccess) {
				t.Fatalf("exit code = %d, want %d", code, exitSuccess)
			}
			if stderr.String() != "" {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if strings.Contains(stdout.String(), "\n  ") {
				t.Fatalf("%s should use compact JSON, got: %q", flag, stdout.String())
			}
			var result commandResult
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("%s stdout was not valid JSON: %v\nstdout: %s", flag, err, stdout.String())
			}
			if result.Status != "pass" || result.ExitCode != int(exitSuccess) || result.EvidenceRefs == nil {
				t.Fatalf("%s dropped required fields: %#v", flag, result)
			}
		})
	}
}

func TestUnknownCommandReturnsMachineReadableInvalidUsage(t *testing.T) {
	code, result, _, stderr := runCommand(t, "nope")

	if code != int(exitInvalidUsage) {
		t.Fatalf("exit code = %d, want %d", code, exitInvalidUsage)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if result.Status != "error" || result.ExitCode != int(exitInvalidUsage) {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("errors length = %d, want 1", len(result.Errors))
	}
	if result.Errors[0].Type != "invalid_usage" || result.Errors[0].ExitCode != int(exitInvalidUsage) {
		t.Fatalf("unexpected error detail: %#v", result.Errors[0])
	}
}

func TestUnknownOptionReturnsMachineReadableInvalidUsage(t *testing.T) {
	code, result, _, _ := runCommand(t, "--bogus")

	if code != int(exitInvalidUsage) {
		t.Fatalf("exit code = %d, want %d", code, exitInvalidUsage)
	}
	if result.Command != "status" {
		t.Fatalf("command = %q, want status", result.Command)
	}
	if got := result.Errors[0].Message; !strings.Contains(got, "unknown option") {
		t.Fatalf("message = %q, want unknown option", got)
	}
}

func TestHumanErrorWritesToStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"nope"}, &stdout, &stderr, true)

	if code != int(exitInvalidUsage) {
		t.Fatalf("exit code = %d, want %d", code, exitInvalidUsage)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, `unknown command "nope"`) || !strings.Contains(got, "(exit 2)") {
		t.Fatalf("stderr = %q, want human error with exit code", got)
	}
}

func TestKnownMVPCommandStubReturnsTypedNotImplemented(t *testing.T) {
	code, result, _, _ := runCommand(t, "backtest", "--window", "180d")

	if code != int(exitGeneral) {
		t.Fatalf("exit code = %d, want %d", code, exitGeneral)
	}
	if result.Command != "backtest" || result.Status != "error" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("errors length = %d, want 1", len(result.Errors))
	}
	if result.Errors[0].Type != "not_implemented" {
		t.Fatalf("error type = %q, want not_implemented", result.Errors[0].Type)
	}
	if len(result.Errors[0].EvidenceRefs) == 0 {
		t.Fatalf("not implemented error must preserve evidence refs")
	}
}

func TestModelsWithoutPullReturnsInvalidUsage(t *testing.T) {
	code, result, _, _ := runCommand(t, "models")

	if code != int(exitInvalidUsage) {
		t.Fatalf("exit code = %d, want %d", code, exitInvalidUsage)
	}
	if result.Command != "models" {
		t.Fatalf("command = %q, want models", result.Command)
	}
	if got := result.Errors[0].Message; !strings.Contains(got, "requires") {
		t.Fatalf("message = %q, want requires", got)
	}
}

func TestModelsPullSkeletonUsesFullCommandName(t *testing.T) {
	code, result, _, _ := runCommand(t, "models", "pull")

	if code != int(exitGeneral) {
		t.Fatalf("exit code = %d, want %d", code, exitGeneral)
	}
	if result.Command != "models pull" {
		t.Fatalf("command = %q, want models pull", result.Command)
	}
	if result.Mode != "models_pull" {
		t.Fatalf("mode = %q, want models_pull", result.Mode)
	}
}

func TestStatusRejectsUnexpectedArguments(t *testing.T) {
	code, result, _, _ := runCommand(t, "status", "extra")

	if code != int(exitInvalidUsage) {
		t.Fatalf("exit code = %d, want %d", code, exitInvalidUsage)
	}
	if result.Command != "status" || !strings.Contains(result.Errors[0].Message, "does not accept arguments") {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestWriteFailureReturnsGeneralExit(t *testing.T) {
	var stderr bytes.Buffer

	code := run([]string{"status"}, failingWriter{}, &stderr, false)

	if code != int(exitGeneral) {
		t.Fatalf("exit code = %d, want %d", code, exitGeneral)
	}
	if got := stderr.String(); !strings.Contains(got, "failed to write command result") {
		t.Fatalf("stderr = %q, want write failure", got)
	}
}

func TestIsTerminalReturnsFalseForRegularAndClosedFiles(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "relia-terminal-test")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if isTerminal(file) {
		t.Fatalf("regular temp file should not be treated as a terminal")
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	if isTerminal(file) {
		t.Fatalf("closed file should not be treated as a terminal")
	}
}

func TestExitCodeExamplesCoverStableExitContract(t *testing.T) {
	exampleDir := filepath.Join("..", "..", "examples", "command-results")
	entries, err := os.ReadDir(exampleDir)
	if err != nil {
		t.Fatalf("read examples: %v", err)
	}

	seen := map[int]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(exampleDir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		var result commandResult
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("%s is not valid command result JSON: %v", entry.Name(), err)
		}
		if result.ObjectType != "relia.command_result" {
			t.Fatalf("%s object_type = %q", entry.Name(), result.ObjectType)
		}
		if result.SchemaVersion != commandResultSchemaVersion {
			t.Fatalf("%s schema_version = %q", entry.Name(), result.SchemaVersion)
		}
		if result.ExitCode < int(exitSuccess) || result.ExitCode > int(exitProvenanceIntegrity) {
			t.Fatalf("%s exit_code = %d, outside stable contract", entry.Name(), result.ExitCode)
		}
		if result.ExitCode == int(exitSuccess) && result.Status != "pass" {
			t.Fatalf("%s exit 0 status = %q, want pass", entry.Name(), result.Status)
		}
		if result.ExitCode != int(exitSuccess) && (result.Status != "error" || len(result.Errors) == 0) {
			t.Fatalf("%s nonzero exit must include machine-readable errors: %#v", entry.Name(), result)
		}
		seen[result.ExitCode] = true
	}

	for code := int(exitSuccess); code <= int(exitProvenanceIntegrity); code++ {
		if !seen[code] {
			t.Fatalf("missing command-result example for exit code %d", code)
		}
	}
}
