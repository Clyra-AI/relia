package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Clyra-AI/relia/internal/yamlmini"
)

func TestInitDiscoversRequiredChecksAndCompileTargets(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "go.mod"), "module github.com/Clyra-AI/relia\n\ngo 1.26.5\n")
	writeFileForTest(t, filepath.Join(tempDir, ".github", "required-checks.json"), `{"required_checks":["validate","CodeQL analyze"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "init"}, false)
	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	configContent, err := os.ReadFile(filepath.Join(tempDir, "relia.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := yamlmini.ParseDocument(string(configContent))
	if err != nil {
		t.Fatalf("relia.yaml did not parse: %v\n%s", err, configContent)
	}
	if got := yamlmini.ListValues(document, "outcomes.checks.required"); strings.Join(got, ",") != "validate,CodeQL analyze" {
		t.Fatalf("discovered checks = %#v", got)
	}
	if got := yamlmini.ListValues(document, "serve.compile.targets"); strings.Join(got, ",") != "AGENTS.md,CLAUDE.md" {
		t.Fatalf("compile targets = %#v", got)
	}
	if got := document.Scalars["serve.compile.max_rules"].Value; got != "25" {
		t.Fatalf("compile max_rules = %q", got)
	}
	if got := strings.Join(yamlmini.ListValues(document, "attribution.coauthor_trailers"), ","); got != "Claude,Claude Code" {
		t.Fatalf("coauthor attribution markers = %#v", got)
	}
	if got := strings.Join(yamlmini.ListValues(document, "attribution.pr_labels"), ","); got != "agent-authored" {
		t.Fatalf("label attribution markers = %#v", got)
	}
}

func TestCompileWritesManagedBlocksIdempotentlyAndPreservesOutsideMarkers(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def rollover():\n    pass\n")
	writeFileForTest(t, filepath.Join(tempDir, "packages", "search", "query.py"), "def query():\n    pass\n")
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "active.yaml"), compileActiveRuleYAML())
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "candidate.yaml"), compileCandidateRuleYAML())
	agentsBefore := "manual header\n<!-- relia:begin (generated; edit rules in memory/rules/, not here) -->\nstale generated text\n<!-- relia:end -->\nmanual footer\n"
	writeFileForTest(t, filepath.Join(tempDir, "AGENTS.md"), agentsBefore)
	writeFileForTest(t, filepath.Join(tempDir, "CLAUDE.md"), "claude manual preface\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "compile"}, false)
	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "compile" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	if got := int(result.Data["changed_targets"].(float64)); got != 3 {
		t.Fatalf("changed_targets = %d, want AGENTS, CLAUDE, and compiled block", got)
	}
	if result.Data["agent_access_boundary"].(map[string]any)["active_memory_only"] != true {
		t.Fatalf("agent_access_boundary = %#v", result.Data["agent_access_boundary"])
	}

	agentsAfter := readTextForTest(t, filepath.Join(tempDir, "AGENTS.md"))
	if !strings.HasPrefix(agentsAfter, "manual header\n") || !strings.HasSuffix(agentsAfter, "\nmanual footer\n") {
		t.Fatalf("AGENTS.md outside marker content changed:\n%s", agentsAfter)
	}
	for _, want := range []string{
		"<!-- relia:begin (generated; edit rules in memory/rules/, not here) -->",
		"<!-- relia:end -->",
		"billing-active",
		"Use invoice clock fixtures instead of direct datetime calls.",
		"https://github.com/acme/billing/pull/42",
		"Non-MCP agents should treat only the active accepted rules below as Relia memory.",
	} {
		if !strings.Contains(agentsAfter, want) {
			t.Fatalf("AGENTS.md missing %q:\n%s", want, agentsAfter)
		}
	}
	if strings.Contains(agentsAfter, "search-candidate") || strings.Contains(agentsAfter, "stale generated text") {
		t.Fatalf("AGENTS.md included inactive or stale generated content:\n%s", agentsAfter)
	}
	if strings.Contains(agentsAfter, "): >") {
		t.Fatalf("AGENTS.md rendered unresolved block-scalar marker:\n%s", agentsAfter)
	}
	claudeAfter := readTextForTest(t, filepath.Join(tempDir, "CLAUDE.md"))
	if !strings.HasPrefix(claudeAfter, "claude manual preface\n\n<!-- relia:begin") {
		t.Fatalf("CLAUDE.md did not append managed block after manual content:\n%s", claudeAfter)
	}
	compiledBlock := readTextForTest(t, filepath.Join(tempDir, "memory", "compiled", "agents-block.md"))
	if !strings.Contains(compiledBlock, "billing-active") || strings.Contains(compiledBlock, "search-candidate") {
		t.Fatalf("compiled block content = %q", compiledBlock)
	}
	if !bytes.Contains([]byte(stdout), []byte("schemas/compiled-context.schema.json")) {
		t.Fatalf("compile output missing compiled-context evidence ref: %s", stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "compile"}, false)
	if code != ExitSuccess {
		t.Fatalf("second compile exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	second := decodeResult(t, stdout)
	if got := int(second.Data["changed_targets"].(float64)); got != 0 {
		t.Fatalf("second compile changed_targets = %d, want idempotent zero", got)
	}
	if got := readTextForTest(t, filepath.Join(tempDir, "AGENTS.md")); got != agentsAfter {
		t.Fatalf("AGENTS.md changed on idempotent second compile")
	}
}

func TestCompileRejectsBrokenManagedMarkers(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def rollover():\n    pass\n")
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "active.yaml"), compileActiveRuleYAML())
	writeFileForTest(t, filepath.Join(tempDir, "AGENTS.md"), "manual\n<!-- relia:begin (generated; edit rules in memory/rules/, not here) -->\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "compile"}, false)
	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "managed marker") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func readTextForTest(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func compileActiveRuleYAML() string {
	return `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-active
kind: avoid
status: active
statement: >
  Use invoice clock fixtures instead of direct datetime calls.
scope:
  paths:
    - packages/billing/invoice.py
confidence: 0.92
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0042
provenance:
  - pr: 42
    outcome: ci_failure
    url: https://github.com/acme/billing/pull/42
review:
  label: accepted
  gate: human_review
  decision: approved
  statement_origin: human_authored
metadata:
  confidence_label: high
  lifecycle_reason: accepted test rule
`
}

func compileCandidateRuleYAML() string {
	return `object_type: relia.memory_rule
schema_version: "1.0"
id: search-candidate
kind: avoid
status: candidate
statement: Candidate search rule should not compile.
scope:
  paths:
    - packages/search/query.py
confidence: 0.5
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0043
provenance:
  - pr: 43
    outcome: ci_failure
    url: https://github.com/acme/billing/pull/43
review:
  label: suggested
  statement_origin: human_authored
metadata:
  confidence_label: medium
  lifecycle_reason: pending review
`
}
