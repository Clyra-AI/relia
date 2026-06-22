package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestJSONFlagEmitsStableEnvelope(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.ObjectType != "relia.command_result" {
		t.Fatalf("object_type = %q", result.ObjectType)
	}
	if result.SchemaVersion != "1.0" {
		t.Fatalf("schema_version = %q", result.SchemaVersion)
	}
	if result.Command != "check" {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
	if result.ExitCode != ExitSuccess {
		t.Fatalf("exit_code = %d", result.ExitCode)
	}
	if len(result.EvidenceRefs) == 0 {
		t.Fatal("expected evidence_refs to be preserved")
	}
	if result.RedactionStatus == "" {
		t.Fatal("expected redaction_status")
	}
	if result.Metadata["schema_ref"] != "schemas/command-result.schema.json" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestPipedOutputDefaultsToJSON(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "check" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
}

func TestInteractiveOutputIsHumanReadable(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"check"}, true)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	if json.Valid([]byte(stdout)) {
		t.Fatalf("interactive output should be human-readable, got JSON: %s", stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("pass check")) {
		t.Fatalf("interactive output = %q, want pass check", stdout)
	}
}

func TestQuietAndCompactPreserveMachineReadableEnvelope(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--quiet", "--compact", "check"}, true)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	if bytes.Contains([]byte(stdout), []byte("\n  ")) {
		t.Fatalf("compact output should not be indented: %q", stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
	if result.ExitCode != ExitSuccess {
		t.Fatalf("exit_code = %d", result.ExitCode)
	}
	if len(result.EvidenceRefs) == 0 {
		t.Fatal("quiet/compact output dropped evidence_refs")
	}
}

func TestUnknownCommandReturnsTypedUsageError(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "unknown-command"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "error" {
		t.Fatalf("status = %q", result.Status)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if result.Errors[0].Type != "invalid_usage" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if result.Errors[0].ExitCode != ExitUsage {
		t.Fatalf("error exit code = %d", result.Errors[0].ExitCode)
	}
}

func TestUnknownFlagReturnsTypedUsageError(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "--bogus"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestHelpAndVersionUseEnvelope(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "--help"},
		{"--json", "help"},
		{"--json", "--version"},
	} {
		stdout, stderr, code := runForTest(t, args, false)
		if code != ExitSuccess {
			t.Fatalf("%v exit code = %d, stderr = %q, stdout = %q", args, code, stderr, stdout)
		}
		result := decodeResult(t, stdout)
		if result.Status != "pass" {
			t.Fatalf("%v status = %q", args, result.Status)
		}
		if len(result.EvidenceRefs) == 0 {
			t.Fatalf("%v dropped evidence refs", args)
		}
	}
}

func TestReservedCommandsReturnTypedNotImplemented(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "ingest"},
		{"--json", "models", "pull"},
	} {
		stdout, stderr, code := runForTest(t, args, false)
		if code != ExitInternal {
			t.Fatalf("%v exit code = %d, stderr = %q, stdout = %q", args, code, stderr, stdout)
		}
		result := decodeResult(t, stdout)
		if result.Errors[0].Type != "not_implemented" {
			t.Fatalf("%v error type = %q", args, result.Errors[0].Type)
		}
	}
}

func TestModelsRejectsUnsupportedSubcommand(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "models"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestInitCreatesBaselineConfig(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	stdout, stderr, code := runForTest(t, []string{"--json", "init"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "init" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	configPath := filepath.Join(tempDir, "relia.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected relia.yaml to be created: %v", err)
	}
	for _, token := range []string{"version: 1", "schema_version: \"1.0\"", "local_only: true", "fail_closed: true", "embeddings: signature", "advisory_only: true"} {
		if !bytes.Contains(content, []byte(token)) {
			t.Fatalf("relia.yaml missing %q:\n%s", token, content)
		}
	}
	for _, dir := range artifactSkeletonDirs {
		if info, err := os.Stat(filepath.Join(tempDir, dir)); err != nil || !info.IsDir() {
			t.Fatalf("expected artifact skeleton dir %s: info=%#v err=%v", dir, info, err)
		}
	}
}

func TestInitRejectsPositionalArguments(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "init", "extra"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestInitExistingConfigIsIdempotent(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	if err := os.WriteFile("relia.yaml", []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "init"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if created, ok := result.Data["created"].(bool); !ok || created {
		t.Fatalf("created = %#v, want false", result.Data["created"])
	}
}

func TestCheckFailsOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "local_configuration_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckReportsMissingOperatingPackFiles(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	if err := os.WriteFile("go.mod", []byte("module github.com/Clyra-AI/relia\n\ngo 1.26.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("relia.yaml", []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "operating_pack_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestHumanErrorWritesToStderr(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"unknown-command"}, true)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !bytes.Contains([]byte(stderr), []byte("error unknown-command")) {
		t.Fatalf("stderr = %q, want human error", stderr)
	}
}

func TestLowLevelHelpers(t *testing.T) {
	tempFile, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer tempFile.Close()

	if stdoutIsTerminal(tempFile) {
		t.Fatal("temporary file should not be detected as a terminal")
	}
	errResult := internalError("failed", errors.New("boom"))
	if errResult.ExitCode != ExitInternal || !bytes.Contains([]byte(errResult.Message), []byte("boom")) {
		t.Fatalf("internal error = %#v", errResult)
	}
}

func TestCommandResultExitCodeExamplesCoverStableCodes(t *testing.T) {
	root := findRepoRootForTest(t)
	content, err := os.ReadFile(filepath.Join(root, "examples", "command-results", "exit-code-examples.json"))
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		ObjectType    string          `json:"object_type"`
		SchemaVersion string          `json:"schema_version"`
		Examples      []CommandResult `json:"examples"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode examples: %v", err)
	}
	if payload.ObjectType != "relia.command_result_examples" {
		t.Fatalf("object_type = %q", payload.ObjectType)
	}
	codes := make([]int, 0, len(payload.Examples))
	for _, example := range payload.Examples {
		if example.ObjectType != "relia.command_result" {
			t.Fatalf("example object_type = %q", example.ObjectType)
		}
		if example.SchemaVersion != "1.0" {
			t.Fatalf("example schema_version = %q", example.SchemaVersion)
		}
		if example.Metadata["schema_version"] != "1.0" {
			t.Fatalf("example metadata = %#v", example.Metadata)
		}
		if example.ExitCode < ExitSuccess || example.ExitCode > ExitProvenanceIntegrity {
			t.Fatalf("unexpected exit code in example: %d", example.ExitCode)
		}
		codes = append(codes, example.ExitCode)
	}
	sort.Ints(codes)
	want := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	if len(codes) != len(want) {
		t.Fatalf("codes = %v, want %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("codes = %v, want %v", codes, want)
		}
	}
}

func TestPhase0SchemasDeclareMetadata(t *testing.T) {
	root := findRepoRootForTest(t)
	if commandErr := validateSchemaContracts(root); commandErr != nil {
		t.Fatalf("schema contracts failed: %#v", commandErr)
	}
}

func TestCheckReportsPhase0ContractRefs(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got, ok := result.Data["schema_contracts"].(float64); !ok || int(got) != len(requiredSchemaFiles) {
		t.Fatalf("schema_contracts = %#v, want %d", result.Data["schema_contracts"], len(requiredSchemaFiles))
	}
	if result.Data["privacy_default"] != "local_only" {
		t.Fatalf("privacy_default = %#v", result.Data["privacy_default"])
	}
	if len(result.Artifacts) <= len(requiredSchemaFiles) {
		t.Fatalf("expected schema artifacts in result: %#v", result.Artifacts)
	}
}

func TestCheckRejectsUnsafePrivacyConfig(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "send_code: false", "send_code: true")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckFailsClosedForDisabledRedaction(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "fail_closed: true", "fail_closed: false")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "redaction_safety_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckRequiresLocalModelManifest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckRejectsRuleWithoutProvenance(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	rulesDir := filepath.Join(tempDir, "memory", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := `object_type: relia.memory_rule
schema_version: "1.0"
id: avoid-direct-time
kind: avoid
status: active
statement: >
  Do not mock time directly.
scope:
  paths:
    - tests/
confidence: 0.8
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_001
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`
	if err := os.WriteFile(filepath.Join(rulesDir, "avoid-direct-time.yaml"), []byte(rule), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "provenance") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestCheckAcceptsDocumentedScopedConfigAndMemoryRuleListMaps(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "  scopes: []", `  scopes:
    - prefix: packages/billing/
      checks: [pytest-billing]`)

	rulesDir := filepath.Join(tempDir, "memory", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := `object_type: relia.memory_rule
schema_version: "1.0"
id: avoid-direct-time
kind: avoid
status: active
statement: >
  Do not mock time directly.
scope:
  paths:
    - packages/billing/
  signals:
    - pytest-billing
confidence: 0.8
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_001
provenance:
  - pr: 142
    outcome: revert
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`
	if err := os.WriteFile(filepath.Join(rulesDir, "avoid-direct-time.yaml"), []byte(rule), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestYAMLParserRecordsNestedFieldsInListMapItems(t *testing.T) {
	document, err := parseYAMLDocument(`repo:
  scopes:
    - prefix: packages/billing/
      checks:
        - pytest-billing
provenance:
  - pr: 142
    outcome: revert
`)
	if err != nil {
		t.Fatalf("parseYAMLDocument returned error: %v", err)
	}

	scopes := document.ListMaps["repo.scopes"]
	if len(scopes) != 1 {
		t.Fatalf("repo.scopes list maps = %#v", scopes)
	}
	if got := scopes[0]["prefix"].Value; got != "packages/billing/" {
		t.Fatalf("scope prefix = %q", got)
	}
	if _, ok := scopes[0]["checks"]; !ok {
		t.Fatalf("scope list-map fields = %#v, want checks container", scopes[0])
	}
	scopeChecks := document.Lists["repo.scopes[0].checks"]
	if len(scopeChecks) != 1 || scopeChecks[0].Value != "pytest-billing" {
		t.Fatalf("scope checks = %#v", scopeChecks)
	}

	provenance := document.ListMaps["provenance"]
	if len(provenance) != 1 {
		t.Fatalf("provenance list maps = %#v", provenance)
	}
	if got := provenance[0]["pr"].Value; got != "142" {
		t.Fatalf("provenance pr = %q", got)
	}
	if got := provenance[0]["outcome"].Value; got != "revert" {
		t.Fatalf("provenance outcome = %q", got)
	}
}

func TestCheckRejectsMemoryRuleMissingSchemaRequiredFields(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	rulesDir := filepath.Join(tempDir, "memory", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := `schema_version: "1.0"
id: avoid-direct-time
kind: avoid
status: active
statement: >
  Do not mock time directly.
confidence: 0.8
evidence:
  experiences:
    - exp_001
provenance:
  - pr: 142
    outcome: revert
review:
  label: accepted
`
	if err := os.WriteFile(filepath.Join(rulesDir, "avoid-direct-time.yaml"), []byte(rule), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "object_type") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestCheckRejectsScalarMemoryRuleProvenanceEntry(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	rulesDir := filepath.Join(tempDir, "memory", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := `object_type: relia.memory_rule
schema_version: "1.0"
id: avoid-direct-time
kind: avoid
status: active
statement: >
  Do not mock time directly.
scope:
  paths:
    - tests/
confidence: 0.8
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_001
provenance:
  - pr-142
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`
	if err := os.WriteFile(filepath.Join(rulesDir, "avoid-direct-time.yaml"), []byte(rule), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "provenance") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func runForTest(t *testing.T, args []string, stdoutIsTTY bool) (string, string, int) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(args, &stdout, &stderr, stdoutIsTTY)
	return stdout.String(), stderr.String(), code
}

func decodeResult(t *testing.T, output string) CommandResult {
	t.Helper()

	var result CommandResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode command result from %q: %v", output, err)
	}
	return result
}

func findRepoRootForTest(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, ok := findRepoRoot(wd)
	if !ok {
		t.Fatalf("could not find repo root from %s", wd)
	}
	return root
}

func setupContractRepo(t *testing.T) string {
	t.Helper()

	sourceRoot := findRepoRootForTest(t)
	tempDir := t.TempDir()
	files := map[string]string{
		"AGENTS.md":              "repo contract\n",
		"WORKFLOW.md":            "workflow contract\n",
		"README.md":              "readme\n",
		"Makefile":               "prepush-full:\n",
		".tool-versions":         "golang 1.26.4\n",
		"go.mod":                 "module github.com/Clyra-AI/relia\n\ngo 1.26.4\n",
		"relia.yaml":             defaultConfigYAML(),
		"docs/product/prd.md":    "prd\n",
		"docs/dev/dev_guides.md": "dev guides\n",
		"docs/architecture/architecture_guides.md": "architecture guides\n",
		".github/required-checks.json":             "{}\n",
		".github/workflows/validate.yml":           "name: validate\n",
		".github/workflows/codeql.yml":             "name: codeql\n",
		".factory/factoryd.example.json":           "{}\n",
		".factory/factoryd.autoship.example.json":  "{}\n",
	}
	for rel, content := range files {
		writeFileForTest(t, filepath.Join(tempDir, rel), content)
	}
	for _, rel := range requiredSchemaFiles {
		content, err := os.ReadFile(filepath.Join(sourceRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		writeFileForTest(t, filepath.Join(tempDir, rel), string(content))
	}
	return tempDir
}

func writeFileForTest(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replaceInFile(t *testing.T, path string, old string, new string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	next := strings.Replace(string(content), old, new, 1)
	if next == string(content) {
		t.Fatalf("expected to replace %q in %s", old, path)
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		t.Fatal(err)
	}
}
