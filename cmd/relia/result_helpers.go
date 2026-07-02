package main

import (
	"time"

	resultdoc "github.com/Clyra-AI/relia/internal/result"
)

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
	return resultdoc.Pass(command, mode, message, start, data, commandResultBuildOptions())
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
	return resultdoc.Error(command, mode, commandErr, start, commandResultBuildOptions())
}

func errorResultWithData(command string, mode string, commandErr *CommandError, start time.Time, data map[string]any) CommandResult {
	return resultdoc.ErrorWithData(command, mode, commandErr, start, data, commandResultBuildOptions())
}
