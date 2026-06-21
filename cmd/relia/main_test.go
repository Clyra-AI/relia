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
	if result.Metadata["schema_id"] != "schemas/command-result.schema.json" {
		t.Fatalf("metadata.schema_id = %#v", result.Metadata["schema_id"])
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
	for _, token := range []string{
		"version: 1",
		`schema_version: "1.0"`,
		"embeddings: signature",
		"entropy_scan: true",
		"fail_closed: true",
		"commit_experiences: false",
		"share_scope: private",
		"org_eligible: false",
		"advisory_only: true",
		"advise:",
		"enabled: false",
		"max_comments_per_pr: 1",
		"badge:",
		"stale_after_days: 30",
		"stale_after_merged_prs: 20",
	} {
		if !bytes.Contains(content, []byte(token)) {
			t.Fatalf("relia.yaml missing %q:\n%s", token, content)
		}
	}
	for _, rel := range []string{
		filepath.Join(".relia", "experiences"),
		filepath.Join(".relia", "signatures"),
		filepath.Join(".relia", "coverage"),
		filepath.Join(".relia", "reports"),
		filepath.Join(".relia", "baselines"),
		filepath.Join("memory", "rules"),
		filepath.Join("memory", "compiled"),
	} {
		info, err := os.Stat(filepath.Join(tempDir, rel))
		if err != nil {
			t.Fatalf("expected init artifact directory %s: %v", rel, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", rel)
		}
	}
	ignoreContent, err := os.ReadFile(filepath.Join(tempDir, ".relia", ".gitignore"))
	if err != nil {
		t.Fatalf("expected experience cache ignore rule: %v", err)
	}
	if !bytes.Contains(ignoreContent, []byte("/experiences/")) {
		t.Fatalf(".relia/.gitignore = %q, want /experiences/", ignoreContent)
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

func TestInitDoesNotIgnoreExperiencesWhenExistingConfigOptsIn(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	config := strings.Replace(defaultConfigYAML(), "commit_experiences: false", "commit_experiences: true", 1)
	if err := os.WriteFile("relia.yaml", []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "init"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", ".gitignore")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("experience cache ignore rule err = %v, want not exist", err)
	}
}

func TestCheckReportsSchemaContractsAndPrivacyDefaults(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "check" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	artifactContract, ok := result.Data["artifact_contract"].(map[string]any)
	if !ok {
		t.Fatalf("artifact_contract missing from data: %#v", result.Data)
	}
	if artifactContract["schema_version"] != "1.0" {
		t.Fatalf("artifact_contract.schema_version = %#v", artifactContract["schema_version"])
	}
	if artifactContract["generated_root"] != ".relia" {
		t.Fatalf("artifact_contract.generated_root = %#v", artifactContract["generated_root"])
	}
	if artifactContract["user_memory_root"] != "memory" {
		t.Fatalf("artifact_contract.user_memory_root = %#v", artifactContract["user_memory_root"])
	}
	schemas, ok := artifactContract["schemas"].([]any)
	if !ok || len(schemas) < len(requiredSchemaFiles) {
		t.Fatalf("schemas = %#v", artifactContract["schemas"])
	}
	privacy, ok := result.Data["privacy_defaults"].(map[string]any)
	if !ok {
		t.Fatalf("privacy_defaults missing from data: %#v", result.Data)
	}
	for key, want := range map[string]any{
		"redaction_fail_closed": true,
		"entropy_scan":          true,
		"embeddings":            "signature",
		"commit_experiences":    false,
		"share_scope":           "private",
		"org_eligible":          false,
		"advisory_only":         true,
		"provider_configured":   false,
	} {
		if privacy[key] != want {
			t.Fatalf("privacy_defaults[%s] = %#v, want %#v", key, privacy[key], want)
		}
	}
}

func TestCheckRejectsConfiguredRepoRootsOutsideContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
	}{
		{
			name:   "artifact root",
			config: strings.Replace(defaultConfigYAML(), "artifact_root: .relia", "artifact_root: artifacts", 1),
		},
		{
			name:   "memory root",
			config: strings.Replace(defaultConfigYAML(), "memory_root: memory", "memory_root: docs/memory", 1),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := writeMinimalRepoForCheck(t, tc.config)
			t.Chdir(repo)

			stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

			if code != ExitValidation {
				t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if result.Errors[0].Type != "memory_artifact_validation_failed" {
				t.Fatalf("error type = %q", result.Errors[0].Type)
			}
		})
	}
}

func TestCheckRejectsUnknownDistillProvider(t *testing.T) {
	repo := writeMinimalRepoForCheck(t, strings.Replace(defaultConfigYAML(), "embeddings: signature", "provider: mystery-ai\n  embeddings: signature", 1))
	t.Chdir(repo)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "local_configuration_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckRejectsDisabledReviewGate(t *testing.T) {
	repo := writeMinimalRepoForCheck(t, strings.Replace(defaultConfigYAML(), "review_required: true", "review_required: false", 1))
	t.Chdir(repo)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "local_configuration_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckFailsClosedWhenRedactionDefaultsAreUnsafe(t *testing.T) {
	config := strings.NewReplacer(
		"entropy_scan: true", "entropy_scan: false",
		"fail_closed: true", "fail_closed: false",
	).Replace(defaultConfigYAML())
	repo := writeMinimalRepoForCheck(t, config)
	t.Chdir(repo)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "redaction_safety_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if result.RedactionStatus != "failed_closed" {
		t.Fatalf("redaction_status = %q", result.RedactionStatus)
	}
}

func TestCheckRejectsNonPrivateShareScope(t *testing.T) {
	repo := writeMinimalRepoForCheck(t, strings.Replace(defaultConfigYAML(), "share_scope: private", "share_scope: org", 1))
	t.Chdir(repo)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "redaction_safety_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if result.RedactionStatus != "failed_closed" {
		t.Fatalf("redaction_status = %q", result.RedactionStatus)
	}
}

func TestCheckRejectsLocalEmbeddingsWithoutArtifact(t *testing.T) {
	repo := writeMinimalRepoForCheck(t, strings.Replace(defaultConfigYAML(), "embeddings: signature", "embeddings: local", 1))
	t.Chdir(repo)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
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
		Metadata      map[string]any  `json:"metadata"`
		Examples      []CommandResult `json:"examples"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode examples: %v", err)
	}
	if payload.ObjectType != "relia.command_result_examples" {
		t.Fatalf("object_type = %q", payload.ObjectType)
	}
	if payload.Metadata["schema_id"] != "schemas/command-result.schema.json" {
		t.Fatalf("metadata.schema_id = %#v", payload.Metadata["schema_id"])
	}
	codes := make([]int, 0, len(payload.Examples))
	for _, example := range payload.Examples {
		if example.ObjectType != "relia.command_result" {
			t.Fatalf("example object_type = %q", example.ObjectType)
		}
		if example.SchemaVersion != "1.0" {
			t.Fatalf("example schema_version = %q", example.SchemaVersion)
		}
		if example.ExitCode < ExitSuccess || example.ExitCode > ExitProvenanceIntegrity {
			t.Fatalf("unexpected exit code in example: %d", example.ExitCode)
		}
		if example.Metadata["schema_id"] != "schemas/command-result.schema.json" {
			t.Fatalf("example %d metadata.schema_id = %#v", example.ExitCode, example.Metadata["schema_id"])
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

func TestSchemaFilesDeclareVersionMetadataAndExitMapping(t *testing.T) {
	root := findRepoRootForTest(t)
	for _, rel := range requiredSchemaFiles {
		content, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(content, &schema); err != nil {
			t.Fatalf("%s is not valid JSON: %v", rel, err)
		}
		if schema["type"] != "object" {
			t.Fatalf("%s type = %#v", rel, schema["type"])
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s properties missing", rel)
		}
		if _, ok := properties["schema_version"]; !ok {
			t.Fatalf("%s missing schema_version property", rel)
		}
		if _, ok := properties["metadata"]; !ok {
			t.Fatalf("%s missing metadata property", rel)
		}
		required, ok := schema["required"].([]any)
		if !ok {
			t.Fatalf("%s required missing", rel)
		}
		if !containsStringValue(required, "schema_version") || !containsStringValue(required, "metadata") {
			t.Fatalf("%s required = %#v", rel, required)
		}
		if _, ok := schema["x-relia_error_mapping"].(map[string]any); !ok {
			t.Fatalf("%s missing x-relia_error_mapping", rel)
		}
	}
}

func TestReliaConfigSchemaAllowsDocumentedAdvisorySections(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/relia-config.schema.json")
	properties := schemaPropertiesForTest(t, "schemas/relia-config.schema.json", schema)

	for _, section := range []string{"advise", "badge"} {
		property := propertyForTest(t, properties, section)
		if property["type"] != "object" {
			t.Fatalf("%s type = %#v, want object", section, property["type"])
		}
	}
}

func TestDefaultConfigIncludesDocumentedAdvisorySections(t *testing.T) {
	config := defaultConfigYAML()
	for _, token := range []string{
		"advise:",
		"enabled: false",
		"max_comments_per_pr: 1",
		"update_in_place: true",
		"reassess_debounce_minutes: 10",
		"min_confidence: 0.6",
		"badge:",
		"stale_after_days: 30",
		"stale_after_merged_prs: 20",
	} {
		if !strings.Contains(config, token) {
			t.Fatalf("default config missing %q:\n%s", token, config)
		}
	}
}

func TestReliaConfigSchemaRequiresReviewGate(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/relia-config.schema.json")
	properties := schemaPropertiesForTest(t, "schemas/relia-config.schema.json", schema)
	distill := propertyForTest(t, properties, "distill")
	distillProperties := schemaPropertiesForTest(t, "relia_config.distill", distill)
	reviewRequired := propertyForTest(t, distillProperties, "review_required")

	if reviewRequired["const"] != true {
		t.Fatalf("distill.review_required = %#v, want const true", reviewRequired)
	}
}

func TestOutcomeSchemasUsePRDOutcomeTaxonomy(t *testing.T) {
	root := findRepoRootForTest(t)
	experienceSchema := readSchemaForTest(t, root, "schemas/experience-record.schema.json")
	experienceProperties := schemaPropertiesForTest(t, "schemas/experience-record.schema.json", experienceSchema)
	outcomeProperties := schemaPropertiesForTest(t, "experience.outcome", propertyForTest(t, experienceProperties, "outcome"))
	kindEnum := enumForTest(t, "experience.outcome.kind", propertyForTest(t, outcomeProperties, "kind"))
	for _, want := range []string{"ci_failure", "revert", "review_correction", "merge_clean", "fix_held"} {
		if !containsStringValue(kindEnum, want) {
			t.Fatalf("experience outcome enum = %#v, want %s", kindEnum, want)
		}
	}
	for _, forbidden := range []string{"ci_failed", "reverted", "merged_clean"} {
		if containsStringValue(kindEnum, forbidden) {
			t.Fatalf("experience outcome enum = %#v, must use PRD outcome names", kindEnum)
		}
	}

	flakeDiscount := propertyForTest(t, experienceProperties, "flake_discount")
	if flakeDiscount["type"] != "number" {
		t.Fatalf("flake_discount type = %#v, want number", flakeDiscount["type"])
	}
	if flakeDiscount["minimum"] != float64(0) || flakeDiscount["maximum"] != float64(1) {
		t.Fatalf("flake_discount bounds = [%#v, %#v], want [0, 1]", flakeDiscount["minimum"], flakeDiscount["maximum"])
	}

	outcomeSchema := readSchemaForTest(t, root, "schemas/outcome-evidence.schema.json")
	outcomeEvidenceProperties := schemaPropertiesForTest(t, "schemas/outcome-evidence.schema.json", outcomeSchema)
	outcomeKindEnum := enumForTest(t, "outcome_evidence.outcome_kind", propertyForTest(t, outcomeEvidenceProperties, "outcome_kind"))
	for _, want := range []string{"ci_failure", "revert", "review_correction", "merge_clean", "fix_held"} {
		if !containsStringValue(outcomeKindEnum, want) {
			t.Fatalf("outcome evidence enum = %#v, want %s", outcomeKindEnum, want)
		}
	}
	for _, forbidden := range []string{"ci_failed", "reverted", "merged_clean"} {
		if containsStringValue(outcomeKindEnum, forbidden) {
			t.Fatalf("outcome evidence enum = %#v, must use PRD outcome names", outcomeKindEnum)
		}
	}
}

func TestFailureSignatureSchemaUsesPRDSignatureClasses(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/failure-signature.schema.json")
	properties := schemaPropertiesForTest(t, "schemas/failure-signature.schema.json", schema)
	signatureClassEnum := enumForTest(t, "failure_signature.signature_class", propertyForTest(t, properties, "signature_class"))

	for _, want := range []string{"test_failure", "lint_failure", "type_failure", "build_failure"} {
		if !containsStringValue(signatureClassEnum, want) {
			t.Fatalf("signature_class enum = %#v, want %s", signatureClassEnum, want)
		}
	}
	if containsStringValue(signatureClassEnum, "typecheck_failure") {
		t.Fatalf("signature_class enum = %#v, must use PRD type_failure", signatureClassEnum)
	}
}

func TestMemoryRuleSchemaMatchesPRDArtifactShape(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/memory-rule.schema.json")
	properties := schemaPropertiesForTest(t, "schemas/memory-rule.schema.json", schema)
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("memory-rule required missing")
	}

	for _, want := range []string{"id", "status", "evidence", "provenance"} {
		if !containsStringValue(required, want) {
			t.Fatalf("memory-rule required = %#v, want %s", required, want)
		}
	}
	for _, forbidden := range []string{"rule_id", "lifecycle"} {
		if containsStringValue(required, forbidden) {
			t.Fatalf("memory-rule required = %#v, must use PRD artifact field names", required)
		}
		if _, ok := properties[forbidden]; ok {
			t.Fatalf("memory-rule properties include %s, must use PRD artifact field names", forbidden)
		}
	}

	statusEnum := enumForTest(t, "memory_rule.status", propertyForTest(t, properties, "status"))
	for _, want := range []string{"candidate", "active", "stale", "contradicted", "retired"} {
		if !containsStringValue(statusEnum, want) {
			t.Fatalf("memory rule status enum = %#v, want %s", statusEnum, want)
		}
	}

	evidence := propertyForTest(t, properties, "evidence")
	evidenceRequired, ok := evidence["required"].([]any)
	if !ok {
		t.Fatal("memory-rule evidence required missing")
	}
	for _, want := range []string{"count", "experiences"} {
		if !containsStringValue(evidenceRequired, want) {
			t.Fatalf("memory-rule evidence required = %#v, want %s", evidenceRequired, want)
		}
	}
	evidenceProperties := schemaPropertiesForTest(t, "memory_rule.evidence", evidence)
	experiences := propertyForTest(t, evidenceProperties, "experiences")
	if experiences["minItems"] != float64(1) {
		t.Fatalf("memory-rule evidence experiences minItems = %#v, want 1", experiences["minItems"])
	}

	provenance := propertyForTest(t, properties, "provenance")
	if provenance["type"] != "array" || provenance["minItems"] != float64(1) {
		t.Fatalf("memory-rule provenance = %#v, want non-empty array", provenance)
	}
	items, ok := provenance["items"].(map[string]any)
	if !ok {
		t.Fatal("memory-rule provenance items missing")
	}
	provenanceRequired, ok := items["required"].([]any)
	if !ok {
		t.Fatal("memory-rule provenance item required missing")
	}
	for _, want := range []string{"pr", "outcome"} {
		if !containsStringValue(provenanceRequired, want) {
			t.Fatalf("memory-rule provenance required = %#v, want %s", provenanceRequired, want)
		}
	}
	provenanceProperties := schemaPropertiesForTest(t, "memory_rule.provenance.item", items)
	provenanceOutcomeEnum := enumForTest(t, "memory_rule.provenance.outcome", propertyForTest(t, provenanceProperties, "outcome"))
	for _, want := range []string{"ci_failure", "revert", "review_correction", "merge_clean", "fix_held"} {
		if !containsStringValue(provenanceOutcomeEnum, want) {
			t.Fatalf("memory-rule provenance outcome enum = %#v, want %s", provenanceOutcomeEnum, want)
		}
	}
	for _, forbidden := range []string{"reverted", "merged_clean"} {
		if containsStringValue(provenanceOutcomeEnum, forbidden) {
			t.Fatalf("memory-rule provenance outcome enum = %#v, must use PRD outcome names", provenanceOutcomeEnum)
		}
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

func containsStringValue(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func readSchemaForTest(t *testing.T, root string, rel string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(content, &schema); err != nil {
		t.Fatalf("%s is not valid JSON: %v", rel, err)
	}
	return schema
}

func schemaPropertiesForTest(t *testing.T, name string, schema map[string]any) map[string]any {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s properties missing", name)
	}
	return properties
}

func propertyForTest(t *testing.T, properties map[string]any, name string) map[string]any {
	t.Helper()
	property, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("%s property missing", name)
	}
	return property
}

func enumForTest(t *testing.T, name string, property map[string]any) []any {
	t.Helper()
	values, ok := property["enum"].([]any)
	if !ok {
		t.Fatalf("%s enum missing", name)
	}
	return values
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

func writeMinimalRepoForCheck(t *testing.T, reliaYAML string) string {
	t.Helper()

	root := t.TempDir()
	for _, rel := range requiredCheckFiles {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := []byte("placeholder\n")
		switch rel {
		case "go.mod":
			content = []byte("module github.com/Clyra-AI/relia\n\ngo 1.26.4\n")
		case defaultConfigFile:
			content = []byte(reliaYAML)
		case ".tool-versions":
			content = []byte("golang 1.26.4\n")
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
