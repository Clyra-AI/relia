package result

import (
	"time"

	configdoc "github.com/Clyra-AI/relia/internal/config"
)

const (
	ObjectType = "relia.command_result"
	SchemaRef  = "schemas/command-result.schema.json"
)

type BuildOptions struct {
	SchemaVersion           string
	ReliaVersion            string
	SuccessExitCode         int
	RedactionSafetyExitCode int
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
	MachineReadable bool           `json:"-"`
}

type Finding = configdoc.Finding

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

func Pass(command string, mode string, message string, start time.Time, data map[string]any, options BuildOptions) CommandResult {
	if data == nil {
		data = map[string]any{}
	}
	data["message"] = message
	return Base(command, mode, "pass", options.SuccessExitCode, start, data, options)
}

func Error(command string, mode string, commandErr *CommandError, start time.Time, options BuildOptions) CommandResult {
	result := Base(command, mode, "error", commandErr.ExitCode, start, nil, options)
	result.Errors = append(result.Errors, *commandErr)
	if commandErr.ExitCode == options.RedactionSafetyExitCode {
		result.RedactionStatus = "failed_closed"
	}
	return result
}

func ErrorWithData(command string, mode string, commandErr *CommandError, start time.Time, data map[string]any, options BuildOptions) CommandResult {
	result := Error(command, mode, commandErr, start, options)
	result.Data = data
	return result
}

func Base(command string, mode string, status string, exitCode int, start time.Time, data map[string]any, options BuildOptions) CommandResult {
	return CommandResult{
		ObjectType:    ObjectType,
		SchemaVersion: options.SchemaVersion,
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
			SchemaRef,
		},
		DurationMS:      time.Since(start).Milliseconds(),
		RedactionStatus: "not_applicable",
		Metadata: map[string]any{
			"relia_version":  options.ReliaVersion,
			"schema_ref":     SchemaRef,
			"schema_version": options.SchemaVersion,
		},
		Data: data,
	}
}
