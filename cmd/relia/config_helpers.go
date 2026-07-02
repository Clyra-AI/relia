package main

import (
	assessdoc "github.com/Clyra-AI/relia/internal/assess"
	configdoc "github.com/Clyra-AI/relia/internal/config"
	memorydoc "github.com/Clyra-AI/relia/internal/memory"
	resultdoc "github.com/Clyra-AI/relia/internal/result"
	reviewdoc "github.com/Clyra-AI/relia/internal/review"
	"github.com/Clyra-AI/relia/internal/yamlmini"
)

func commandResultBuildOptions() resultdoc.BuildOptions {
	return resultdoc.BuildOptions{
		SchemaVersion:           commandSchemaVersion,
		ReliaVersion:            reliaVersion,
		SuccessExitCode:         ExitSuccess,
		RedactionSafetyExitCode: ExitRedactionSafety,
	}
}

func assessmentBuildOptions() assessdoc.Options {
	return assessdoc.Options{
		SchemaVersion:            commandSchemaVersion,
		ArtifactContractError:    artifactContractError,
		InternalError:            internalError,
		ProvenanceIntegrityError: provenanceIntegrityError,
		RepoPathExists:           configdoc.RepoPathExists,
		YAMLFloat:                yamlFloat,
	}
}

func validateReliaConfig(root string) ([]Finding, *CommandError) {
	return validateReliaConfigWithEmbeddingOverride(root, "")
}

func validateReliaConfigForDistill(root string, embeddingOverride string) ([]Finding, *CommandError) {
	return validateReliaConfigWithEmbeddingOverride(root, embeddingOverride)
}

func validateReliaConfigWithEmbeddingOverride(root string, embeddingOverride string) ([]Finding, *CommandError) {
	warnings, configErr := configdoc.Validate(root, configdoc.ValidationOptions{
		SchemaVersion:     commandSchemaVersion,
		ReliaVersion:      reliaVersion,
		EmbeddingOverride: embeddingOverride,
	})
	return warnings, commandErrorFromConfig(configErr)
}

func validateAdviseConfig(document yamlDocument) *CommandError {
	_, commandErr := adviseSettingsFromConfig(document)
	return commandErr
}

func adviseSettingsFromConfig(document yamlDocument) (adviseSettings, *CommandError) {
	settings, configErr := configdoc.AdviseSettingsFromConfig(document)
	return settings, commandErrorFromConfig(configErr)
}

func validateLocalModelManifest(root string, manifestScalar yamlScalar) *CommandError {
	return commandErrorFromConfig(configdoc.ValidateLocalModelManifest(root, manifestScalar))
}

func validateLocalModelManifestPayload(root string, manifest configdoc.LocalModelManifest, ref string) *CommandError {
	return commandErrorFromConfig(configdoc.ValidateLocalModelManifestPayload(root, manifest, ref))
}

func validateSchemaContracts(root string) *CommandError {
	return commandErrorFromConfig(configdoc.ValidateSchemaContracts(root, requiredSchemaFiles))
}

func validateMemoryRuleArtifacts(root string) *CommandError {
	return memorydoc.ValidateRuleArtifacts(root, memoryValidationOptions())
}

func validateMemoryRuleArtifact(root string, path string) *CommandError {
	return memorydoc.ValidateRuleArtifact(root, path, memoryValidationOptions())
}

func validateDraftedMemoryRuleCalibration(document yamlDocument, rel string, confidence float64, evidenceCount int, contradictions int) *CommandError {
	return memorydoc.ValidateDraftedRuleCalibration(document, rel, confidence, evidenceCount, contradictions, memoryValidationOptions())
}

func reviewUpdateOptions() reviewdoc.UpdateOptions {
	return reviewdoc.UpdateOptions{
		SchemaVersion:         commandSchemaVersion,
		UsageError:            usageError,
		ArtifactContractError: artifactContractError,
		InternalError:         internalError,
		RepoPathExists:        configdoc.RepoPathExists,
		YAMLFloat:             yamlFloat,
	}
}

func memoryValidationOptions() memorydoc.ValidationOptions {
	return memorydoc.ValidationOptions{
		SchemaVersion:         commandSchemaVersion,
		ArtifactContractError: artifactContractError,
		InternalError:         internalError,
		RepoPathExists:        configdoc.RepoPathExists,
		YAMLFloat:             yamlFloat,
	}
}

func parseYAMLDocument(content string) (yamlDocument, error) {
	return yamlmini.ParseDocument(content)
}

func hasYAMLPath(document yamlDocument, path string) bool {
	return yamlmini.HasPath(document, path)
}

func leadingSpaces(value string) int {
	return yamlmini.LeadingSpaces(value)
}

func configRef(scalar yamlScalar) string {
	return configdoc.Ref(defaultConfigFile, scalar)
}

func configRefWithPath(path string, scalar yamlScalar) string {
	return configdoc.RefWithPath(path, scalar)
}

func yamlPathRef(document yamlDocument, path string) string {
	return configdoc.PathRef(defaultConfigFile, document, path)
}

func defaultConfigYAML() string {
	return configdoc.DefaultYAML(commandSchemaVersion, reliaVersion)
}
