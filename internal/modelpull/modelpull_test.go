package modelpull

import "testing"

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
