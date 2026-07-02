package modelpull

import (
	"testing"

	"github.com/Clyra-AI/relia/internal/yamlmini"
)

func TestParseArgsBuildsManifestOptions(t *testing.T) {
	options, parseErr := ParseArgs([]string{
		"--model-id", "text-embedding-test",
		"--version", "2026-06-22",
		"--source-url", "https://example.test/model.bin",
		"--license", "Apache-2.0",
		"--digest", "sha256:ABCDEF",
		"--cache-path", ".relia/models/artifact.bin",
		"--update-policy", "manual",
		"--rollback-policy", "delete artifact",
	})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	manifest := Manifest(options, ".relia/models/artifact.bin")
	if manifest.ModelID != "text-embedding-test" ||
		manifest.Digest != "abcdef" ||
		manifest.Status != "ready" ||
		manifest.CachePath != ".relia/models/artifact.bin" {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestParseArgsRejectsUnknownArgument(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--bogus"})
	if parseErr == nil {
		t.Fatal("expected parse error")
	}
	if parseErr.Message != `unknown models pull argument "--bogus"` {
		t.Fatalf("parse error = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsMissingRequiredFlag(t *testing.T) {
	_, parseErr := ParseArgs(nil)
	if parseErr == nil {
		t.Fatal("expected parse error")
	}
	if parseErr.Message != "models pull requires --model-id" {
		t.Fatalf("parse error = %q", parseErr.Message)
	}
}

func TestConfiguredManifestPathUsesConfigScalar(t *testing.T) {
	got := ConfiguredManifestPath(map[string]yamlmini.Scalar{
		"models.local_manifest": {Value: "custom/models.json"},
	})

	if got != "custom/models.json" {
		t.Fatalf("ConfiguredManifestPath = %q", got)
	}
}

func TestPrepareManifestNormalizesPaths(t *testing.T) {
	options := Options{
		ModelID:        "text-embedding-test",
		Version:        "2026-06-22",
		SourceURL:      "https://example.test/model.bin",
		License:        "Apache-2.0",
		Digest:         "sha256:ABCDEF",
		CachePath:      ".relia/models/artifact.bin",
		UpdatePolicy:   "manual",
		RollbackPolicy: "delete artifact",
	}

	manifest, manifestPath, pathErr := PrepareManifest(options, ".relia/models/manifest.json", "relia.yaml")

	if pathErr != nil {
		t.Fatalf("PrepareManifest returned error: %v", pathErr)
	}
	if manifestPath != ".relia/models/manifest.json" {
		t.Fatalf("manifestPath = %q", manifestPath)
	}
	if manifest.CachePath != ".relia/models/artifact.bin" || manifest.Digest != "abcdef" {
		t.Fatalf("manifest = %#v", manifest)
	}
	data := ResultData(manifest, manifestPath)
	if data["manifest_path"] != manifestPath || data["network_used"] != false || data["status"] != "ready" {
		t.Fatalf("ResultData = %#v", data)
	}
}

func TestPrepareManifestRejectsInvalidManifestPath(t *testing.T) {
	_, _, pathErr := PrepareManifest(Options{CachePath: ".relia/models/artifact.bin"}, "../manifest.json", "relia.yaml")

	if pathErr == nil {
		t.Fatal("expected invalid manifest path error")
	}
	if pathErr.Kind != PathErrorDependency || pathErr.Reference != "relia.yaml" {
		t.Fatalf("pathErr = %#v", pathErr)
	}
}

func TestPrepareManifestRejectsCacheManifestCollision(t *testing.T) {
	_, _, pathErr := PrepareManifest(Options{CachePath: ".relia/models/manifest.json"}, ".relia/models/manifest.json", "relia.yaml")

	if pathErr == nil {
		t.Fatal("expected cache collision error")
	}
	if pathErr.Kind != PathErrorUsage {
		t.Fatalf("Kind = %q", pathErr.Kind)
	}
	if pathErr.Message != "models pull --cache-path must not equal the local model manifest path" {
		t.Fatalf("Message = %q", pathErr.Message)
	}
}
