package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	configdoc "github.com/Clyra-AI/relia/internal/config"
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
		{"--json", "demo"},
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

func TestCheckRejectsZeroMatchAttributionConfigWithConcreteRef(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	configPath := filepath.Join(tempDir, "relia.yaml")
	replaceInFile(t, configPath, `  coauthor_trailers:
    - Claude
    - Claude Code`, `  coauthor_trailers: []`)
	replaceInFile(t, configPath, `  pr_labels:
    - agent-authored`, `  pr_labels: []`)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "zero agent matchers") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
	if !strings.Contains(result.Errors[0].Ref, "relia.yaml:") {
		t.Fatalf("error ref = %q, want concrete relia.yaml line", result.Errors[0].Ref)
	}
}

func TestCheckRejectsNaNRecurrenceGateThreshold(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	configPath := filepath.Join(tempDir, "relia.yaml")
	replaceInFile(t, configPath, `gate:
  enabled: false`, `gate:
  enabled: true
  max_error_recurrence_rate: NaN`)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "gate.max_error_recurrence_rate must be a number between 0 and 1") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
	if !strings.Contains(result.Errors[0].Ref, "relia.yaml:") {
		t.Fatalf("error ref = %q, want concrete relia.yaml line", result.Errors[0].Ref)
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
	for _, dir := range []string{".relia/experiences", ".relia/signatures", ".relia/coverage", ".relia/reports", ".relia/baselines", "memory/rules", "memory/compiled"} {
		if info, err := os.Stat(filepath.Join(tempDir, dir)); err != nil || !info.IsDir() {
			t.Fatalf("expected artifact skeleton dir %s: info=%#v err=%v", dir, info, err)
		}
	}
	ignoreContent, err := os.ReadFile(filepath.Join(tempDir, ".gitignore"))
	if err != nil {
		t.Fatalf("expected .gitignore to be created: %v", err)
	}
	if !bytes.Contains(ignoreContent, []byte(".relia/")) {
		t.Fatalf(".gitignore missing .relia/:\n%s", ignoreContent)
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
	if err := os.WriteFile("go.mod", []byte("module github.com/Clyra-AI/relia\n\ngo 1.26.5\n"), 0o644); err != nil {
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

func TestRecurrenceReportSchemaKeepsT8FieldsOptionalForV1(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(findRepoRootForTest(t), "schemas", "recurrence-report.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(content, &schema); err != nil {
		t.Fatal(err)
	}
	requiredValues, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema required field = %#v", schema["required"])
	}
	required := map[string]bool{}
	for _, value := range requiredValues {
		required[fmt.Sprint(value)] = true
	}
	for _, field := range []string{"metrics", "top_repeated_mistakes", "diagnostics", "operator_feedback", "badge"} {
		if required[field] {
			t.Fatalf("%s must stay optional while recurrence-report schema_version remains 1.0", field)
		}
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

func TestCheckRejectsIncompleteLocalModelManifest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), `{
  "model_id": "text-embedding-test"
}
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "missing required field") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestModelsPullRecordsLocalManifestWithoutNetwork(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	artifactContent := []byte("deterministic local model artifact")
	artifactRel := filepath.ToSlash(filepath.Join(".relia", "models", "artifact.bin"))
	writeFileForTest(t, filepath.Join(tempDir, filepath.FromSlash(artifactRel)), string(artifactContent))
	digest := sha256.Sum256(artifactContent)

	stdout, stderr, code := runForTest(t, []string{
		"--json",
		"models",
		"pull",
		"--model-id", "text-embedding-test",
		"--version", "2026-06-22",
		"--source-url", "https://example.test/model.bin",
		"--license", "Apache-2.0",
		"--digest", fmt.Sprintf("%x", digest),
		"--cache-path", artifactRel,
		"--update-policy", "manual",
		"--rollback-policy", "delete artifact and restore signature embeddings",
	}, false)

	if code != ExitSuccess {
		t.Fatalf("models pull exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "models pull" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	manifestPath := filepath.Join(tempDir, ".relia", "models", "manifest.json")
	var manifest map[string]any
	readJSONFileForTest(t, manifestPath, &manifest)
	if manifest["model_id"] != "text-embedding-test" ||
		manifest["version"] != "2026-06-22" ||
		manifest["source_url"] != "https://example.test/model.bin" ||
		manifest["license"] != "Apache-2.0" ||
		manifest["cache_path"] != artifactRel ||
		manifest["update_policy"] != "manual" ||
		manifest["rollback_policy"] == "" ||
		manifest["status"] != "ready" {
		t.Fatalf("manifest = %#v", manifest)
	}
	if commandErr := validateLocalModelManifest(tempDir, yamlScalar{Value: ".relia/models/manifest.json", Line: 1}); commandErr != nil {
		t.Fatalf("manifest did not validate after models pull: %#v", commandErr)
	}
}

func TestModelsPullRejectsCachePathAtManifestPath(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	digest := sha256.Sum256([]byte("manifest collision payload"))

	stdout, stderr, code := runForTest(t, []string{
		"--json",
		"models",
		"pull",
		"--model-id", "text-embedding-test",
		"--version", "2026-06-22",
		"--source-url", "https://example.test/model.bin",
		"--license", "Apache-2.0",
		"--digest", fmt.Sprintf("%x", digest),
		"--cache-path", ".relia/models/manifest.json",
		"--update-policy", "manual",
		"--rollback-policy", "delete artifact and restore signature embeddings",
	}, false)

	if code != ExitUsage {
		t.Fatalf("models pull manifest cache exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" || !strings.Contains(result.Errors[0].Message, "must not equal") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "models", "manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifest path exists after rejected models pull: %v", err)
	}
}

func TestCheckValidatesLocalModelManifestDigest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	artifactContent := []byte("deterministic local model artifact")
	artifactRel := filepath.Join(".relia", "models", "artifact.bin")
	writeFileForTest(t, filepath.Join(tempDir, artifactRel), string(artifactContent))
	digest := sha256.Sum256(artifactContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), fmt.Sprintf(`{
  "model_id": "text-embedding-test",
  "version": "2026-06-22",
  "source_url": "https://example.test/model.bin",
  "license": "Apache-2.0",
  "digest": "%x",
  "cache_path": "%s",
  "update_policy": "manual",
  "rollback_policy": "delete artifact and restore signature embeddings"
}
`, digest, filepath.ToSlash(artifactRel)))

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestCheckRejectsStaleLocalModelManifest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	artifactContent := []byte("deterministic local model artifact")
	artifactRel := filepath.Join(".relia", "models", "artifact.bin")
	writeFileForTest(t, filepath.Join(tempDir, artifactRel), string(artifactContent))
	digest := sha256.Sum256(artifactContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), fmt.Sprintf(`{
  "model_id": "text-embedding-test",
  "version": "2026-06-22",
  "source_url": "https://example.test/model.bin",
  "license": "Apache-2.0",
  "digest": "%x",
  "cache_path": "%s",
  "update_policy": "manual",
  "rollback_policy": "delete artifact and restore signature embeddings",
  "status": "stale"
}
`, digest, filepath.ToSlash(artifactRel)))

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" || !strings.Contains(result.Errors[0].Message, "stale") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestCheckRejectsEscapedLocalModelCachePath(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	artifactContent := []byte("outside model artifact")
	outsideRel := "outside-model.bin"
	writeFileForTest(t, filepath.Join(tempDir, outsideRel), string(artifactContent))
	digest := sha256.Sum256(artifactContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), fmt.Sprintf(`{
  "model_id": "text-embedding-test",
  "version": "2026-06-22",
  "source_url": "https://example.test/model.bin",
  "license": "Apache-2.0",
  "digest": "%x",
  "cache_path": "../%s",
  "update_policy": "manual",
  "rollback_policy": "delete artifact and restore signature embeddings"
}
`, digest, outsideRel))

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "inside the repository") {
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

func decodeBacktestReportFromResult(t *testing.T, result CommandResult) recurrenceReport {
	t.Helper()
	encoded, err := json.Marshal(result.Data["report"])
	if err != nil {
		t.Fatalf("encode nested backtest report: %v", err)
	}
	var report recurrenceReport
	if err := json.Unmarshal(encoded, &report); err != nil {
		t.Fatalf("decode nested backtest report from %s: %v", encoded, err)
	}
	return report
}

func decodeJSONLines(t *testing.T, content string) []map[string]any {
	t.Helper()

	var records []map[string]any
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode JSON line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func loadRuleDocsByKindForTest(t *testing.T, root string) map[string]yamlDocument {
	t.Helper()
	return loadRuleDocsByScalarForTest(t, root, "kind")
}

func loadRuleDocsByStatusForTest(t *testing.T, root string) map[string]yamlDocument {
	t.Helper()
	return loadRuleDocsByScalarForTest(t, root, "status")
}

func findRuleDocByEvidenceForTest(t *testing.T, root string, kind string, experienceIDs []string) yamlDocument {
	t.Helper()
	for _, document := range loadRuleDocsForTest(t, root) {
		if document.Scalars["kind"].Value != kind {
			continue
		}
		if stringSlicesEqual(yamlScalarValuesForTest(document.Lists["evidence.experiences"]), experienceIDs) {
			return document
		}
	}
	t.Fatalf("could not find %s rule with evidence experiences %#v", kind, experienceIDs)
	return yamlDocument{}
}

func distillClusterKeyForTest(kind string, signatureID string, signatureClass string, checkName string, signatureKey string) string {
	return distillClusterKey(experienceRecord{
		Outcome: experienceOutcome{
			Kind: kind,
			Signature: experienceSignature{
				SignatureID: signatureID,
			},
		},
		Metadata: map[string]any{
			"signature": map[string]any{
				"class":      signatureClass,
				"check_name": checkName,
				"key":        signatureKey,
			},
		},
	})
}

func yamlScalarValuesForTest(scalars []yamlScalar) []string {
	values := make([]string, 0, len(scalars))
	for _, scalar := range scalars {
		values = append(values, scalar.Value)
	}
	return values
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assertReportDiagnosticTypes(t *testing.T, diagnostics []reportDiagnostic, wantTypes []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, diagnostic := range diagnostics {
		seen[diagnostic.Type] = true
		if diagnostic.Status == "" || diagnostic.Message == "" || diagnostic.Ref == "" {
			t.Fatalf("diagnostic missing operator-visible details: %#v", diagnostic)
		}
	}
	for _, want := range wantTypes {
		if !seen[want] {
			t.Fatalf("diagnostics missing %q: %#v", want, diagnostics)
		}
	}
}

func loadRuleDocsByScalarForTest(t *testing.T, root string, scalar string) map[string]yamlDocument {
	t.Helper()
	docs := map[string]yamlDocument{}
	for _, document := range loadRuleDocsForTest(t, root) {
		key := document.Scalars[scalar].Value
		if key == "" {
			t.Fatalf("rule missing scalar %s: %#v", scalar, document.Scalars)
		}
		docs[key] = document
	}
	return docs
}

func loadRuleDocsForTest(t *testing.T, root string) []yamlDocument {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "memory", "rules", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("expected generated memory rule YAML files")
	}
	var docs []yamlDocument
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		document := parseRuleDocForTest(t, string(content))
		docs = append(docs, document)
	}
	return docs
}

func readRuleByIDForTest(t *testing.T, root string, ruleID string) string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "memory", "rules", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		document := parseRuleDocForTest(t, string(content))
		if document.Scalars["id"].Value == ruleID {
			return string(content)
		}
	}
	t.Fatalf("could not find generated rule %q", ruleID)
	return ""
}

func parseRuleDocForTest(t *testing.T, content string) yamlDocument {
	t.Helper()
	document, err := parseYAMLDocument(content)
	if err != nil {
		t.Fatalf("parse rule YAML:\n%s\n%v", content, err)
	}
	return document
}

func findRepoRootForTest(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if root, ok := configdoc.FindRepoRoot(wd); ok {
		return root
	}
	t.Fatalf("could not find repo root from %s", wd)
	return ""
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
		".tool-versions":         "golang 1.26.5\n",
		"go.mod":                 "module github.com/Clyra-AI/relia\n\ngo 1.26.5\n",
		"relia.yaml":             defaultConfigYAML(),
		"docs/product/prd.md":    "prd\n",
		"docs/dev/dev_guides.md": "dev guides\n",
		"docs/architecture/architecture_guides.md": "architecture guides\n",
		"packages/billing/.keep":                   "\n",
		"tests/.keep":                              "\n",
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

func enableProviderForTest(t *testing.T, root string, provider string, model string, baseURL string, credentialEnv string, maxCost string) {
	t.Helper()
	replaceInFile(t, filepath.Join(root, "relia.yaml"), `distill:
  embeddings: signature
  provider: none
  model: ""
  base_url: ""
  credential_env: ""
  max_cost_usd_per_run: 0
  input_cost_usd_per_1k_tokens: 0
  output_cost_usd_per_1k_tokens: 0
  review_required: true`, fmt.Sprintf(`distill:
  embeddings: signature
  provider: %s
  model: %s
  base_url: %s
  credential_env: %s
  max_cost_usd_per_run: %s
  input_cost_usd_per_1k_tokens: 0.001
  output_cost_usd_per_1k_tokens: 0.002
  review_required: true`, provider, model, baseURL, credentialEnv, maxCost))
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

func runGitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
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
