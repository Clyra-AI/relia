package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	modelpulldoc "github.com/Clyra-AI/relia/internal/modelpull"
)

func modelsResult(args []string, start time.Time) CommandResult {
	if len(args) == 0 || args[0] != "pull" {
		return errorResult("models", "models", usageError("expected subcommand: pull"), start)
	}
	options, parseErr := modelpulldoc.ParseArgs(args[1:])
	if parseErr != nil {
		return errorResult("models pull", "models", usageError(parseErr.Message), start)
	}
	wd, err := os.Getwd()
	if err != nil {
		return errorResult("models pull", "models", internalError("could not inspect working directory", err), start)
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return errorResult("models pull", "models", configError("could not locate repository root from current directory"), start)
	}
	config, commandErr := readReliaConfig(root)
	if commandErr != nil {
		return errorResult("models pull", "models", commandErr, start)
	}
	manifestRel := modelpulldoc.ConfiguredManifestPath(config.Scalars)
	manifest, manifestDisplayPath, pathErr := modelpulldoc.PrepareManifest(options, manifestRel, defaultConfigFile)
	if pathErr != nil {
		return errorResult("models pull", "models", commandErrorFromModelPullPath(pathErr), start)
	}
	if commandErr := validateLocalModelManifestPayload(root, manifest, manifestRel); commandErr != nil {
		return errorResult("models pull", "models", commandErr, start)
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return errorResult("models pull", "models", internalError("could not encode local model manifest", err), start)
	}
	manifestPath := filepath.Join(root, filepath.FromSlash(manifestDisplayPath))
	if commandErr := writeAtomicRepoFile(manifestPath, append(encoded, '\n'), "local model manifest"); commandErr != nil {
		return errorResult("models pull", "models", commandErr, start)
	}
	result := passResult("models pull", "models", "recorded local embedding model artifact manifest", start, modelpulldoc.ResultData(manifest, manifestDisplayPath))
	result.EvidenceRefs = append(result.EvidenceRefs,
		"docs/dev/dev_guides.md#model-provider-and-artifact-policy",
		manifestDisplayPath,
	)
	result.Artifacts = append(result.Artifacts,
		ArtifactRef{Kind: "local_model_manifest", Path: manifestDisplayPath},
		ArtifactRef{Kind: "local_model_artifact", Path: manifest.CachePath},
	)
	return result
}
