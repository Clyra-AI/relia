package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Clyra-AI/relia/internal/yamlmini"
)

func TestDefaultYAMLUsesVersionPinsAndParses(t *testing.T) {
	content := DefaultYAML("1.0-test", "0.0.0-test")
	document, err := yamlmini.ParseDocument(content)
	if err != nil {
		t.Fatalf("DefaultYAML did not parse: %v", err)
	}
	if got := document.Scalars["artifacts.schema_version"].Value; got != "1.0-test" {
		t.Fatalf("artifacts.schema_version = %q", got)
	}
	if got := document.Scalars["redaction.schema_version"].Value; got != "1.0-test" {
		t.Fatalf("redaction.schema_version = %q", got)
	}
	if got := document.Scalars["artifacts.relia_version"].Value; got != "0.0.0-test" {
		t.Fatalf("artifacts.relia_version = %q", got)
	}
	for _, key := range []string{"privacy.local_only", "redaction.fail_closed", "advise.update_in_place", "gate.enabled"} {
		if _, ok := document.Scalars[key]; !ok {
			t.Fatalf("DefaultYAML missing scalar %q", key)
		}
	}
}

func TestRefHelpersUseLineNumbers(t *testing.T) {
	if got := Ref(DefaultFile, yamlmini.Scalar{}); got != DefaultFile {
		t.Fatalf("Ref without line = %q", got)
	}
	if got := Ref(DefaultFile, yamlmini.Scalar{Line: 12}); got != "relia.yaml:12" {
		t.Fatalf("Ref with line = %q", got)
	}
	if got := RefWithPath("memory/rules/rule.yaml", yamlmini.Scalar{Line: 4}); got != "memory/rules/rule.yaml:4" {
		t.Fatalf("RefWithPath = %q", got)
	}
}

func TestPathRefFindsScalarContainerListAndDescendant(t *testing.T) {
	document, err := yamlmini.ParseDocument(`attribution:
  agent_authors:
    - login: codex
advise:
  max_comments_per_pr: 1
`)
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	if got := PathRef(DefaultFile, document, "advise.max_comments_per_pr"); got != "relia.yaml:5" {
		t.Fatalf("scalar PathRef = %q", got)
	}
	if got := PathRef(DefaultFile, document, "attribution.agent_authors"); got != "relia.yaml:2" {
		t.Fatalf("list PathRef = %q", got)
	}
	descendant := PathRef(DefaultFile, document, "attribution")
	if !strings.HasPrefix(descendant, "relia.yaml:") {
		t.Fatalf("descendant PathRef = %q", descendant)
	}
	if got := PathRef(DefaultFile, document, "missing.path"); got != DefaultFile {
		t.Fatalf("missing PathRef = %q", got)
	}
}

func TestValidateDefaultYAMLPasses(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, DefaultFile), []byte(DefaultYAML("1.0", "0.0.0-dev")), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings, configErr := Validate(root, ValidationOptions{
		SchemaVersion: "1.0",
		ReliaVersion:  "0.0.0-dev",
	})
	if configErr != nil {
		t.Fatalf("Validate returned error: %v", configErr)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestValidateRejectsProviderBaseURLUserInfo(t *testing.T) {
	content := strings.Replace(DefaultYAML("1.0", "0.0.0-dev"), `distill:
  embeddings: signature
  provider: none
  model: ""
  base_url: ""
  credential_env: ""
  max_cost_usd_per_run: 0
  input_cost_usd_per_1k_tokens: 0
  output_cost_usd_per_1k_tokens: 0
  review_required: true`, `distill:
  embeddings: signature
  provider: openai_compatible
  model: gpt-test
  base_url: https://user:secret@example.test
  credential_env: OPENAI_API_KEY
  max_cost_usd_per_run: 1
  input_cost_usd_per_1k_tokens: 0.01
  output_cost_usd_per_1k_tokens: 0.02
  review_required: true`, 1)
	document, err := yamlmini.ParseDocument(content)
	if err != nil {
		t.Fatal(err)
	}

	_, configErr := ValidateDocument(t.TempDir(), document, ValidationOptions{
		SchemaVersion: "1.0",
		ReliaVersion:  "0.0.0-dev",
	})
	if configErr == nil {
		t.Fatal("expected provider base URL error")
	}
	if configErr.Kind != ErrorConfig || !strings.Contains(configErr.Message, "user info") {
		t.Fatalf("config error = %#v", configErr)
	}
}
