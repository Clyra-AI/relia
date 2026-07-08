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
	if got := strings.Join(yamlmini.ListValues(document, "serve.compile.targets"), ","); got != "AGENTS.md,CLAUDE.md" {
		t.Fatalf("compile targets = %q", got)
	}
	if got := document.Scalars["serve.compile.max_rules"].Value; got != "25" {
		t.Fatalf("compile max_rules = %q", got)
	}
}

func TestDefaultYAMLWithChecksRendersDiscoveredChecks(t *testing.T) {
	content := DefaultYAMLWithChecks("1.0", "0.0.0-dev", []string{"validate", "CodeQL analyze", "validate"})
	document, err := yamlmini.ParseDocument(content)
	if err != nil {
		t.Fatalf("DefaultYAMLWithChecks did not parse: %v", err)
	}
	if got := strings.Join(yamlmini.ListValues(document, "outcomes.checks.required"), ","); got != "validate,CodeQL analyze" {
		t.Fatalf("required checks = %q", got)
	}
}

func TestArtifactSkeletonPathsReturnsCopy(t *testing.T) {
	paths := ArtifactSkeletonPaths()
	if len(paths) == 0 {
		t.Fatal("expected artifact skeleton paths")
	}
	paths[0] = "mutated"
	if got := ArtifactSkeletonPaths()[0]; got == "mutated" {
		t.Fatal("ArtifactSkeletonPaths returned mutable package storage")
	}
}

func TestEnsureArtifactSkeletonCreatesDirectories(t *testing.T) {
	root := t.TempDir()

	if err := EnsureArtifactSkeleton(root); err != nil {
		t.Fatalf("EnsureArtifactSkeleton returned error: %v", err)
	}

	for _, dir := range ArtifactSkeletonPaths() {
		if info, err := os.Stat(filepath.Join(root, dir)); err != nil || !info.IsDir() {
			t.Fatalf("expected artifact skeleton dir %s: info=%#v err=%v", dir, info, err)
		}
	}
}

func TestEnsureReliaGitIgnoreCreatesAndPreservesEntries(t *testing.T) {
	root := t.TempDir()

	if err := EnsureReliaGitIgnore(root); err != nil {
		t.Fatalf("EnsureReliaGitIgnore returned error: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("expected .gitignore: %v", err)
	}
	if got := string(content); got != ".relia/\n" {
		t.Fatalf(".gitignore = %q", got)
	}

	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("build/\n.relia/*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureReliaGitIgnore(root); err != nil {
		t.Fatalf("EnsureReliaGitIgnore returned error for existing entry: %v", err)
	}
	content, err = os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("expected .gitignore: %v", err)
	}
	if got := string(content); got != "build/\n.relia/*\n" {
		t.Fatalf(".gitignore changed despite existing Relia entry: %q", got)
	}
}

func TestFindRepoRootFindsReliaModuleFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested", "child")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/Clyra-AI/relia\n\ngo 1.26.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := FindRepoRoot(nested)
	if !ok {
		t.Fatal("FindRepoRoot did not find module root")
	}
	if got != root {
		t.Fatalf("FindRepoRoot = %q, want %q", got, root)
	}
}

func TestFindRepoRootRejectsDifferentModule(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/other\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got, ok := FindRepoRoot(root); ok || got != "" {
		t.Fatalf("FindRepoRoot = %q, %v; want no root", got, ok)
	}
}

func TestCleanRepoPathRejectsUnsafeAndNormalizes(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		" ./cmd/relia/main.go ":          {"cmd/relia/main.go", true},
		"cmd/../internal/config/repo.go": {"internal/config/repo.go", true},
		"":                               {"", false},
		"/absolute":                      {"", false},
		"../outside":                     {"", false},
		"cmd/../../outside":              {"", false},
	}

	for input, tc := range cases {
		got, ok := CleanRepoPath(input)
		if ok != tc.ok || filepath.ToSlash(got) != tc.want {
			t.Fatalf("CleanRepoPath(%q) = %q, %v; want %q, %v", input, got, ok, tc.want, tc.ok)
		}
	}
}

func TestScopePatternMatchesDoubleStar(t *testing.T) {
	if !ScopePatternMatches("cmd/**", "cmd/relia/main.go") {
		t.Fatal("expected recursive scope pattern to match nested path")
	}
	if ScopePatternMatches("cmd/**", "internal/config/repo.go") {
		t.Fatal("recursive scope pattern matched outside prefix")
	}
	if !ScopePatternMatches("*.md", "README.md") {
		t.Fatal("expected path.Match-compatible pattern")
	}
}

func TestWorkingTreePathMatchesSkipsInternalStateDirs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "relia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "relia", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".relia", "experiences"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".relia", "experiences", "record.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !WorkingTreePathMatches(root, "cmd/**") {
		t.Fatal("expected working tree glob to match source file")
	}
	if WorkingTreePathMatches(root, ".relia/**") {
		t.Fatal("working tree glob should skip Relia runtime state")
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

func TestDiscoverRequiredChecksUsesManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".github", "required-checks.json"), []byte(`{"required_checks":["validate","CodeQL analyze"]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(DiscoverRequiredChecks(root), ","); got != "validate,CodeQL analyze" {
		t.Fatalf("DiscoverRequiredChecks = %q", got)
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
