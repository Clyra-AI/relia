package config

import (
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
