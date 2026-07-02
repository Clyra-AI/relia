package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSchemaContractsAcceptsRequiredEnvelopeFields(t *testing.T) {
	root := t.TempDir()
	writeSchemaForTest(t, root, "schemas/command-result.schema.json", `{
  "type": "object",
  "required": ["schema_version", "metadata"],
  "properties": {
    "schema_version": {"type": "string"},
    "metadata": {"type": "object"}
  }
}`)

	if err := ValidateSchemaContracts(root, []string{"schemas/command-result.schema.json"}); err != nil {
		t.Fatalf("ValidateSchemaContracts returned error: %v", err)
	}
}

func TestValidateSchemaContractsRejectsMissingMetadataRequirement(t *testing.T) {
	root := t.TempDir()
	writeSchemaForTest(t, root, "schemas/command-result.schema.json", `{
  "type": "object",
  "required": ["schema_version"],
  "properties": {
    "schema_version": {"type": "string"},
    "metadata": {"type": "object"}
  }
}`)

	err := ValidateSchemaContracts(root, []string{"schemas/command-result.schema.json"})

	if err == nil || err.Kind != ErrorArtifactContract || !strings.Contains(err.Message, "must require metadata") {
		t.Fatalf("error = %#v, want metadata requirement artifact error", err)
	}
}

func writeSchemaForTest(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
