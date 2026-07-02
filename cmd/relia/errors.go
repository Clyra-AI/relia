package main

import (
	"strings"

	advisedoc "github.com/Clyra-AI/relia/internal/advise"
	configdoc "github.com/Clyra-AI/relia/internal/config"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
	modelpulldoc "github.com/Clyra-AI/relia/internal/modelpull"
)

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

func configErrorAt(message string, ref string) *CommandError {
	commandErr := configError(message)
	commandErr.Ref = ref
	return commandErr
}

func commandErrorFromConfig(configErr *configdoc.Error) *CommandError {
	if configErr == nil {
		return nil
	}
	switch configErr.Kind {
	case configdoc.ErrorConfig:
		if configErr.Ref == "" {
			return configError(configErr.Message)
		}
		return configErrorAt(configErr.Message, configErr.Ref)
	case configdoc.ErrorArtifactContract:
		return artifactContractError(configErr.Message, configErr.Ref)
	case configdoc.ErrorRedactionSafety:
		return redactionSafetyError(configErr.Message, configErr.Ref)
	case configdoc.ErrorDependency:
		return dependencyError(configErr.Message, configErr.Ref)
	case configdoc.ErrorInternal:
		return internalError(configErr.Message, configErr.Err)
	default:
		return internalError(configErr.Message, configErr.Err)
	}
}

func commandErrorFromIngest(ingestErr *ingestdoc.Error) *CommandError {
	if ingestErr == nil {
		return nil
	}
	switch ingestErr.Kind {
	case ingestdoc.ErrorArtifactContract:
		return artifactContractError(ingestErr.Message, ingestErr.Ref)
	case ingestdoc.ErrorInternal:
		return internalError(ingestErr.Message, nil)
	case ingestdoc.ErrorProvenance:
		return provenanceIntegrityError(ingestErr.Message, ingestErr.Ref)
	case ingestdoc.ErrorRedactionSafety:
		return redactionSafetyError(ingestErr.Message, ingestErr.Ref)
	default:
		return internalError(ingestErr.Message, nil)
	}
}

func commandErrorFromAdviseState(stateErr *advisedoc.StateError) *CommandError {
	if stateErr == nil {
		return nil
	}
	switch stateErr.Kind {
	case advisedoc.StateErrorUsage:
		return usageError(stateErr.Message)
	case advisedoc.StateErrorArtifactContract:
		return artifactContractError(stateErr.Message, stateErr.Ref)
	case advisedoc.StateErrorInternal:
		return internalError(stateErr.Message, stateErr.Err)
	default:
		return internalError(stateErr.Message, stateErr.Err)
	}
}

func commandErrorFromModelPullPath(pathErr *modelpulldoc.PathError) *CommandError {
	if pathErr == nil {
		return nil
	}
	switch pathErr.Kind {
	case modelpulldoc.PathErrorDependency:
		return dependencyError(pathErr.Message, pathErr.Reference)
	case modelpulldoc.PathErrorUsage:
		return usageError(pathErr.Message)
	default:
		return internalError(pathErr.Message, nil)
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

func provenanceIntegrityError(message string, ref string) *CommandError {
	return &CommandError{
		Type:        "provenance_integrity_failed",
		Message:     message,
		ExitCode:    ExitProvenanceIntegrity,
		Remediation: "Provide PR and source evidence URLs before persisting canonical experience records.",
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
