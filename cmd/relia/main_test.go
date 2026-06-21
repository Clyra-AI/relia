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

func TestInitRemovesManagedExperienceIgnoreWhenExistingConfigOptsIn(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	stdout, stderr, code := runForTest(t, []string{"--json", "init"}, false)
	if code != ExitSuccess {
		t.Fatalf("initial init exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	configPath := filepath.Join(tempDir, "relia.yaml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config = []byte(strings.Replace(string(config), "commit_experiences: false", "commit_experiences: true", 1))
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "init"}, false)
	if code != ExitSuccess {
		t.Fatalf("second init exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	ignoreContent, err := os.ReadFile(filepath.Join(tempDir, ".relia", ".gitignore"))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ignoreContent, []byte("/experiences/")) {
		t.Fatalf(".relia/.gitignore = %q, must not ignore opted-in experience shards", ignoreContent)
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

func TestCheckValidatesConfigAgainstSchemaRequiredTopLevelFields(t *testing.T) {
	for _, key := range []string{"metadata", "attribution", "outcomes"} {
		t.Run(key, func(t *testing.T) {
			repo := writeMinimalRepoForConfigSchemaCheck(t, removeTopLevelYAMLBlock(defaultConfigYAML(), key))
			t.Chdir(repo)

			stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

			if code != ExitUsage {
				t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if result.Errors[0].Type != "local_configuration_error" {
				t.Fatalf("error type = %q", result.Errors[0].Type)
			}
			if !strings.Contains(result.Errors[0].Message, key) {
				t.Fatalf("error message = %q, want schema field %q", result.Errors[0].Message, key)
			}
		})
	}
}

func TestCheckValidatesConfigAgainstSchemaNumericBounds(t *testing.T) {
	for _, tc := range []struct {
		name        string
		config      string
		messageWant string
	}{
		{
			name:        "max comments minimum",
			config:      strings.Replace(defaultConfigYAML(), "max_comments_per_pr: 1", "max_comments_per_pr: 0", 1),
			messageWant: "advise.max_comments_per_pr",
		},
		{
			name:        "badge stale days minimum",
			config:      strings.Replace(defaultConfigYAML(), "stale_after_days: 30", "stale_after_days: 0", 1),
			messageWant: "badge.stale_after_days",
		},
		{
			name:        "minimum confidence maximum",
			config:      strings.Replace(defaultConfigYAML(), "min_confidence: 0.6", "min_confidence: 2", 1),
			messageWant: "advise.min_confidence",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := writeMinimalRepoForConfigSchemaCheck(t, tc.config)
			t.Chdir(repo)

			stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

			if code != ExitUsage {
				t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if result.Errors[0].Type != "local_configuration_error" {
				t.Fatalf("error type = %q", result.Errors[0].Type)
			}
			if !strings.Contains(result.Errors[0].Message, tc.messageWant) {
				t.Fatalf("error message = %q, want schema field %q", result.Errors[0].Message, tc.messageWant)
			}
		})
	}
}

func TestCheckAllowsDocumentedNestedBlockLists(t *testing.T) {
	config := strings.NewReplacer(
		"  checks:\n    required: []",
		"  checks:\n    required:\n      - pytest\n      - go test",
		"serve:\n  advisory_only: true",
		"serve:\n  advisory_only: true\n  compile:\n    targets:\n      - AGENTS.md\n      - CLAUDE.md",
	).Replace(defaultConfigYAML())
	repo := writeMinimalRepoForFullCheck(t, config)
	t.Chdir(repo)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestCheckAllowsInlineYAMLSequenceForRedactionPatterns(t *testing.T) {
	config := strings.Replace(defaultConfigYAML(), "patterns:\n    - api_key\n    - token\n    - password\n    - secret\n    - private_key", "patterns: [api_key, token, password, secret, private_key]", 1)
	repo := writeMinimalRepoForFullCheck(t, config)
	t.Chdir(repo)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestCheckAcceptsVersionOnlyPRDBootstrapConfig(t *testing.T) {
	repo := writeMinimalRepoForFullCheck(t, prdBootstrapConfigForTest())
	t.Chdir(repo)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
	privacy, ok := result.Data["privacy_defaults"].(map[string]any)
	if !ok {
		t.Fatalf("privacy_defaults missing from data: %#v", result.Data)
	}
	for key, want := range map[string]any{
		"redaction_fail_closed": true,
		"share_scope":           "private",
		"org_eligible":          false,
		"advisory_only":         true,
	} {
		if privacy[key] != want {
			t.Fatalf("privacy_defaults[%s] = %#v, want %#v", key, privacy[key], want)
		}
	}
}

func TestCheckValidatesActiveMemoryRuleContractBeforePass(t *testing.T) {
	for _, tc := range []struct {
		name        string
		rule        string
		messageWant string
	}{
		{
			name:        "missing provenance",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "provenance:\n  - pr: 123\n    outcome: ci_failure\n    url: https://example.invalid/pr/123\n", "", 1),
			messageWant: "provenance",
		},
		{
			name:        "missing experience citations",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "  experiences:\n    - exp-123\n", "", 1),
			messageWant: "experience citations",
		},
		{
			name:        "missing object type",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "object_type: relia.memory_rule\n", "", 1),
			messageWant: "object_type",
		},
		{
			name:        "missing schema version",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "schema_version: \"1.0\"\n", "", 1),
			messageWant: "schema_version",
		},
		{
			name:        "missing kind",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "kind: avoid\n", "", 1),
			messageWant: "kind",
		},
		{
			name:        "missing confidence",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "confidence: 0.82\n", "", 1),
			messageWant: "confidence",
		},
		{
			name:        "missing scope",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "scope:\n  paths:\n    - cmd/relia/**\n", "", 1),
			messageWant: "scope",
		},
		{
			name:        "missing metadata",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "metadata:\n  relia_version: 0.0.0-dev\n", "", 1),
			messageWant: "metadata",
		},
		{
			name:        "missing provenance outcome",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "    outcome: ci_failure\n", "", 1),
			messageWant: "provenance outcome",
		},
		{
			name:        "inline provenance without pr",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "provenance:\n  - pr: 123\n    outcome: ci_failure\n    url: https://example.invalid/pr/123\n", "provenance: [outcome: ci_failure]\n", 1),
			messageWant: "provenance pr",
		},
		{
			name:        "missing active review label",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "  label: accepted\n", "", 1),
			messageWant: "review label",
		},
		{
			name:        "rejected active review label",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "  label: accepted\n", "  label: rejected\n", 1),
			messageWant: "accepted",
		},
		{
			name:        "needs input active review label",
			rule:        strings.Replace(validMemoryRuleYAMLForTest(), "  label: accepted\n", "  label: needs_user_input\n", 1),
			messageWant: "accepted",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := writeMinimalRepoForFullCheck(t, defaultConfigYAML())
			writeMemoryRuleForTest(t, repo, tc.rule)
			t.Chdir(repo)

			stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

			if code != ExitValidation {
				t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if result.Errors[0].Type != "memory_artifact_validation_failed" {
				t.Fatalf("error type = %q", result.Errors[0].Type)
			}
			if !strings.Contains(result.Errors[0].Message, tc.messageWant) {
				t.Fatalf("error message = %q, want %q", result.Errors[0].Message, tc.messageWant)
			}
			if result.Errors[0].Ref != filepath.Join("memory", "rules", "active-rule.yaml") {
				t.Fatalf("error ref = %q", result.Errors[0].Ref)
			}
		})
	}
}

func TestCheckAcceptsValidActiveMemoryRule(t *testing.T) {
	repo := writeMinimalRepoForFullCheck(t, defaultConfigYAML())
	writeMemoryRuleForTest(t, repo, validMemoryRuleYAMLForTest())
	t.Chdir(repo)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
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

func TestCheckRejectsProviderEmbeddingsWithoutCompleteEndpointGrant(t *testing.T) {
	config := strings.Replace(defaultConfigYAML(), "embeddings: signature", "provider: anthropic\n  embeddings: provider", 1)
	repo := writeMinimalRepoForFullCheck(t, config)
	t.Chdir(repo)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "model_provider_endpoint") {
		t.Fatalf("error message = %q, want model_provider_endpoint", result.Errors[0].Message)
	}
}

func TestCheckAcceptsProviderEmbeddingsWithCompleteEndpointGrant(t *testing.T) {
	config := strings.Replace(defaultConfigYAML(), "embeddings: signature", `provider: anthropic
  model: claude-fable-5
  base_url: https://api.anthropic.example
  credential_env: ANTHROPIC_API_KEY
  budget_posture: max_usd_per_run_2
  redaction_posture: redacted_records_only
  allowlist:
    - api.anthropic.example
  embeddings: provider`, 1)
	repo := writeMinimalRepoForFullCheck(t, config)
	t.Chdir(repo)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
	if len(result.Warnings) == 0 || result.Warnings[0].Type != "provider_path_configured" {
		t.Fatalf("warnings = %#v, want provider_path_configured", result.Warnings)
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

func TestCheckFailsClosedWhenOrgSharingIsEnabled(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "boolean true", value: "true"},
		{name: "non false scalar", value: "yes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := writeMinimalRepoForCheck(t, strings.Replace(defaultConfigYAML(), "org_eligible: false", "org_eligible: "+tc.value, 1))
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
		})
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

func TestReliaConfigSchemaDefinesProviderEndpointGrantFields(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/relia-config.schema.json")
	properties := schemaPropertiesForTest(t, "schemas/relia-config.schema.json", schema)
	distillProperties := schemaPropertiesForTest(t, "relia_config.distill", propertyForTest(t, properties, "distill"))

	for _, want := range []string{"provider", "model", "endpoint", "base_url", "credential_env", "budget_posture", "redaction_posture", "allowlist"} {
		if _, ok := distillProperties[want]; !ok {
			t.Fatalf("relia_config.distill properties = %#v, want %s", distillProperties, want)
		}
	}
	allowlist := propertyForTest(t, distillProperties, "allowlist")
	if allowlist["type"] != "array" || allowlist["minItems"] != float64(1) {
		t.Fatalf("distill.allowlist = %#v, want non-empty array", allowlist)
	}
}

func TestOutcomeSchemasUsePRDOutcomeTaxonomy(t *testing.T) {
	root := findRepoRootForTest(t)
	experienceSchema := readSchemaForTest(t, root, "schemas/experience-record.schema.json")
	experienceProperties := schemaPropertiesForTest(t, "schemas/experience-record.schema.json", experienceSchema)
	repo := propertyForTest(t, experienceProperties, "repo")
	if repo["type"] != "string" || repo["minLength"] != float64(1) {
		t.Fatalf("experience repo = %#v, want non-empty canonical repo string", repo)
	}

	attributionProperties := schemaPropertiesForTest(t, "experience.attribution", propertyForTest(t, experienceProperties, "attribution"))
	attributionRequired, ok := propertyForTest(t, experienceProperties, "attribution")["required"].([]any)
	if !ok {
		t.Fatal("experience.attribution required missing")
	}
	if !containsAnyString(attributionRequired, "agent_authored") {
		t.Fatalf("experience.attribution required = %#v, want agent_authored", attributionRequired)
	}
	agentAuthored := propertyForTest(t, attributionProperties, "agent_authored")
	if agentAuthored["type"] != "boolean" {
		t.Fatalf("experience.attribution.agent_authored type = %#v, want boolean", agentAuthored["type"])
	}

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

func TestExperienceRecordSchemaAcceptsCanonicalActionNames(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/experience-record.schema.json")
	experienceProperties := schemaPropertiesForTest(t, "schemas/experience-record.schema.json", schema)
	action := propertyForTest(t, experienceProperties, "action")
	actionProperties := schemaPropertiesForTest(t, "experience.action", action)
	actionRequired, ok := action["required"].([]any)
	if !ok {
		t.Fatal("experience.action required missing")
	}

	for _, want := range []string{"pr", "commits"} {
		if !containsStringValue(actionRequired, want) {
			t.Fatalf("experience.action required = %#v, want %s", actionRequired, want)
		}
		if _, ok := actionProperties[want]; !ok {
			t.Fatalf("experience.action properties = %#v, want %s", actionProperties, want)
		}
	}
}

func TestExperienceRecordSchemaAcceptsCanonicalOutcomeTerminal(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/experience-record.schema.json")
	experienceProperties := schemaPropertiesForTest(t, "schemas/experience-record.schema.json", schema)
	outcome := propertyForTest(t, experienceProperties, "outcome")
	outcomeProperties := schemaPropertiesForTest(t, "experience.outcome", outcome)
	outcomeRequired, ok := outcome["required"].([]any)
	if !ok {
		t.Fatal("experience.outcome required missing")
	}

	if !containsStringValue(outcomeRequired, "terminal") {
		t.Fatalf("experience.outcome required = %#v, want terminal", outcomeRequired)
	}
	if _, ok := outcomeProperties["terminal"]; !ok {
		t.Fatalf("experience.outcome properties = %#v, want terminal", outcomeProperties)
	}
	if containsStringValue(outcomeRequired, "terminal_state") {
		t.Fatalf("experience.outcome required = %#v, must not require non-canonical terminal_state", outcomeRequired)
	}
}

func TestExperienceRecordSchemaPreservesEmbeddedSignatureDetails(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/experience-record.schema.json")
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("experience schema definitions missing")
	}
	signature, ok := defs["signature_ref"].(map[string]any)
	if !ok {
		t.Fatal("signature_ref definition missing")
	}
	signatureRequired, ok := signature["required"].([]any)
	if !ok {
		t.Fatal("signature_ref required missing")
	}
	for _, want := range []string{"key", "message_fingerprint", "extraction_confidence"} {
		if !containsStringValue(signatureRequired, want) {
			t.Fatalf("signature_ref required = %#v, want %s", signatureRequired, want)
		}
	}
	signatureProperties := schemaPropertiesForTest(t, "experience.signature_ref", signature)
	for _, want := range []string{"signature_id", "signature_class", "check_name", "key", "message_fingerprint", "extraction_confidence"} {
		if _, ok := signatureProperties[want]; !ok {
			t.Fatalf("signature_ref properties = %#v, want %s", signatureProperties, want)
		}
	}
	signatureClassEnum := enumForTest(t, "experience.signature_ref.signature_class", propertyForTest(t, signatureProperties, "signature_class"))
	if !containsStringValue(signatureClassEnum, "type_failure") {
		t.Fatalf("signature_ref signature_class enum = %#v, want type_failure", signatureClassEnum)
	}
}

func TestRecurrenceReportSchemaCapsErrorRecurrenceRate(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/recurrence-report.schema.json")
	properties := schemaPropertiesForTest(t, "schemas/recurrence-report.schema.json", schema)
	headlineProperties := schemaPropertiesForTest(t, "recurrence_report.headline", propertyForTest(t, properties, "headline"))
	errRate := propertyForTest(t, headlineProperties, "error_recurrence_rate")

	if errRate["type"] != "number" {
		t.Fatalf("error_recurrence_rate type = %#v, want number", errRate["type"])
	}
	if errRate["minimum"] != float64(0) || errRate["maximum"] != float64(1) {
		t.Fatalf("error_recurrence_rate bounds = [%#v, %#v], want [0, 1]", errRate["minimum"], errRate["maximum"])
	}
}

func TestRecurrenceReportSchemaPreservesFlakeAndAttributionCounts(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/recurrence-report.schema.json")
	properties := schemaPropertiesForTest(t, "schemas/recurrence-report.schema.json", schema)
	headline := propertyForTest(t, properties, "headline")
	headlineRequired, ok := headline["required"].([]any)
	if !ok {
		t.Fatal("recurrence_report.headline required missing")
	}
	headlineProperties := schemaPropertiesForTest(t, "recurrence_report.headline", headline)

	for _, want := range []string{"attribution_uncertain_count", "flake_discounted_count"} {
		if !containsStringValue(headlineRequired, want) {
			t.Fatalf("recurrence_report.headline required = %#v, want %s", headlineRequired, want)
		}
		property := propertyForTest(t, headlineProperties, want)
		if property["type"] != "integer" || property["minimum"] != float64(0) {
			t.Fatalf("recurrence_report.headline.%s = %#v, want non-negative integer", want, property)
		}
	}
}

func TestRiskAssessmentSchemaIncludesMatchedRulesAndCoverageStats(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/risk-assessment.schema.json")
	properties := schemaPropertiesForTest(t, "schemas/risk-assessment.schema.json", schema)
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("risk-assessment required missing")
	}
	for _, want := range []string{"matched_rules", "coverage_stats"} {
		if !containsStringValue(required, want) {
			t.Fatalf("risk-assessment required = %#v, want %s", required, want)
		}
	}

	matchedRules := propertyForTest(t, properties, "matched_rules")
	if matchedRules["type"] != "array" {
		t.Fatalf("matched_rules type = %#v, want array", matchedRules["type"])
	}
	matchedRuleItem, ok := matchedRules["items"].(map[string]any)
	if !ok {
		t.Fatal("matched_rules.items missing")
	}
	matchedRuleRequired, ok := matchedRuleItem["required"].([]any)
	if !ok {
		t.Fatal("matched_rules item required missing")
	}
	for _, want := range []string{"rule_id", "match_level", "confidence", "citations"} {
		if !containsStringValue(matchedRuleRequired, want) {
			t.Fatalf("matched_rules item required = %#v, want %s", matchedRuleRequired, want)
		}
	}
	matchedRuleProperties := schemaPropertiesForTest(t, "risk_assessment.matched_rules.item", matchedRuleItem)
	citations := propertyForTest(t, matchedRuleProperties, "citations")
	if citations["type"] != "array" || citations["minItems"] != float64(1) {
		t.Fatalf("matched_rules.citations = %#v, want non-empty array", citations)
	}

	coverageStats := propertyForTest(t, properties, "coverage_stats")
	coverageRequired, ok := coverageStats["required"].([]any)
	if !ok {
		t.Fatal("coverage_stats required missing")
	}
	for _, want := range []string{"coverage_status", "path_count", "paths_with_experience", "experience_density", "path_coverage"} {
		if !containsStringValue(coverageRequired, want) {
			t.Fatalf("coverage_stats required = %#v, want %s", coverageRequired, want)
		}
	}
	coverageProperties := schemaPropertiesForTest(t, "risk_assessment.coverage_stats", coverageStats)
	coverageStatusEnum := enumForTest(t, "risk_assessment.coverage_stats.coverage_status", propertyForTest(t, coverageProperties, "coverage_status"))
	for _, want := range []string{"no_coverage", "covered_clean", "covered_risky"} {
		if !containsStringValue(coverageStatusEnum, want) {
			t.Fatalf("coverage_status enum = %#v, want %s", coverageStatusEnum, want)
		}
	}
	experienceDensity := propertyForTest(t, coverageProperties, "experience_density")
	if experienceDensity["type"] != "number" || experienceDensity["minimum"] != float64(0) || experienceDensity["maximum"] != float64(1) {
		t.Fatalf("experience_density = %#v, want bounded number", experienceDensity)
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

func TestMemoryRuleSchemaRequiresAcceptedReviewForActiveStatus(t *testing.T) {
	root := findRepoRootForTest(t)
	schema := readSchemaForTest(t, root, "schemas/memory-rule.schema.json")
	allOf, ok := schema["allOf"].([]any)
	if !ok {
		t.Fatal("memory-rule allOf missing")
	}

	for _, rawClause := range allOf {
		clause, ok := rawClause.(map[string]any)
		if !ok {
			continue
		}
		ifClause, ok := clause["if"].(map[string]any)
		if !ok {
			continue
		}
		ifProperties := schemaPropertiesForTest(t, "memory_rule.allOf.if", ifClause)
		status, ok := ifProperties["status"].(map[string]any)
		if !ok || status["const"] != "active" {
			continue
		}
		thenClause, ok := clause["then"].(map[string]any)
		if !ok {
			t.Fatal("active status conditional missing then clause")
		}
		thenProperties := schemaPropertiesForTest(t, "memory_rule.allOf.then", thenClause)
		review := propertyForTest(t, thenProperties, "review")
		reviewProperties := schemaPropertiesForTest(t, "memory_rule.allOf.then.review", review)
		label := propertyForTest(t, reviewProperties, "label")
		if label["const"] != "accepted" {
			t.Fatalf("active review label const = %#v, want accepted", label["const"])
		}
		return
	}

	t.Fatal("memory-rule schema does not constrain active rules to accepted review labels")
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

func writeMinimalRepoForConfigSchemaCheck(t *testing.T, reliaYAML string) string {
	t.Helper()

	sourceRoot := findRepoRootForTest(t)
	root := writeMinimalRepoForCheck(t, reliaYAML)
	for _, rel := range []string{"schemas/relia-config.schema.json"} {
		sourceContent, err := os.ReadFile(filepath.Join(sourceRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		targetPath := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(targetPath, sourceContent, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeMinimalRepoForFullCheck(t *testing.T, reliaYAML string) string {
	t.Helper()

	sourceRoot := findRepoRootForTest(t)
	root := writeMinimalRepoForCheck(t, reliaYAML)
	for _, rel := range requiredSchemaFiles {
		sourceContent, err := os.ReadFile(filepath.Join(sourceRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		targetPath := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(targetPath, sourceContent, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeMemoryRuleForTest(t *testing.T, root string, content string) {
	t.Helper()

	path := filepath.Join(root, "memory", "rules", "active-rule.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func validMemoryRuleYAMLForTest() string {
	return `object_type: relia.memory_rule
schema_version: "1.0"
id: active-rule
kind: avoid
status: active
statement: >
  Keep reviewed memory rules tied to explicit experiences and PR provenance.
confidence: 0.82
evidence:
  count: 1
  experiences:
    - exp-123
review:
  label: accepted
  reviewed_by: relia-test
scope:
  paths:
    - cmd/relia/**
provenance:
  - pr: 123
    outcome: ci_failure
    url: https://example.invalid/pr/123
metadata:
  relia_version: 0.0.0-dev
`
}

func prdBootstrapConfigForTest() string {
	return `version: 1

repo:
  provider: github
  remote: origin
  scopes: []

attribution:
  agent_authors:
    - login: acme-claude-bot
  coauthor_trailers:
    - "Claude"
    - "Claude Code"
  pr_labels:
    - agent-authored
  uncertain: exclude

outcomes:
  checks:
    required:
      - pytest
      - eslint
  revert_detection: true
  review_corrections:
    marker: "relia:correction"
  lookback_days: 180
  fix_held:
    settle_days: 14
    min_overlapping_merges: 3

redaction:
  patterns:
    - api_key
    - token
    - password
    - secret
  entropy_scan: true

distill:
  provider: anthropic
  model: claude-fable-5
  max_cost_usd_per_run: 2.00
  min_evidence_count: 2
  embeddings: signature
  review_required: true

memory:
  decay_half_life_days: 90
  invalidate_on_path_delete: true
  max_active_rules: 200
  commit_experiences: false

serve:
  mcp: true
  compile:
    targets:
      - AGENTS.md
      - CLAUDE.md
    max_rules: 25

advise:
  enabled: true
  max_comments_per_pr: 1
  update_in_place: true
  reassess_debounce_minutes: 10
  min_confidence: 0.6

badge:
  stale_after_days: 30
  stale_after_merged_prs: 20

gate:
  enabled: false
  max_error_recurrence_rate: null
`
}

func removeTopLevelYAMLBlock(content string, key string) string {
	lines := strings.Split(content, "\n")
	blockStart := key + ":"
	output := make([]string, 0, len(lines))
	skipping := false
	for _, line := range lines {
		if !skipping && strings.HasPrefix(line, blockStart) {
			skipping = true
			continue
		}
		if skipping {
			trimmed := strings.TrimSpace(line)
			indent := len(line) - len(strings.TrimLeft(line, " "))
			if trimmed == "" || indent > 0 {
				continue
			}
			skipping = false
		}
		output = append(output, line)
	}
	return strings.Join(output, "\n")
}
