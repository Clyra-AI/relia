package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestCheckRejectsMemoryRuleWithoutProvenanceURL(t *testing.T) {
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

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "provenance url") {
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
    url: https://github.com/acme/billing-service/pull/142
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

func TestCheckRejectsPlaybookRuleWithoutPositiveEvidence(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	rulesDir := filepath.Join(tempDir, "memory", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := `object_type: relia.memory_rule
schema_version: "1.0"
id: playbook-freeze-time
kind: playbook
status: active
statement: >
  Use the freeze-time fixture for billing rollover tests.
scope:
  paths:
    - packages/billing/
confidence: 0.8
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_held_candidate
provenance:
  - pr: 210
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/210
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`
	if err := os.WriteFile(filepath.Join(rulesDir, "playbook-freeze-time.yaml"), []byte(rule), 0o644); err != nil {
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
	if !strings.Contains(result.Errors[0].Message, "fix_held or merged_clean") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestCheckRejectsActiveUnacceptedMemoryRule(t *testing.T) {
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
  - pr: 142
    outcome: revert
    url: https://github.com/acme/billing-service/pull/142
review:
  label: suggested
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
	if !strings.Contains(result.Errors[0].Message, "review.label must be accepted") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestCheckRejectsMemoryRuleWithoutConcreteScope(t *testing.T) {
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
scope: {}
confidence: 0.8
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_001
provenance:
  - pr: 142
    outcome: revert
    url: https://github.com/acme/billing-service/pull/142
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
	if !strings.Contains(result.Errors[0].Message, "scope path or signal") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestCheckRejectsMemoryRuleWithUnknownScopePath(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	rulesDir := filepath.Join(tempDir, "memory", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := `object_type: relia.memory_rule
schema_version: "1.0"
id: avoid-unknown-path
kind: avoid
status: active
statement: >
  Do not depend on unknown packages.
scope:
  paths:
    - pakcages/billing/
confidence: 0.8
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_001
provenance:
  - pr: 142
    outcome: revert
    url: https://github.com/acme/billing-service/pull/142
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`
	if err := os.WriteFile(filepath.Join(rulesDir, "avoid-unknown-path.yaml"), []byte(rule), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "scope path does not exist") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestCheckAcceptsMemoryRuleWithWorkingTreeGlobScope(t *testing.T) {
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
    - tests/**
confidence: 0.8
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_001
provenance:
  - pr: 142
    outcome: revert
    url: https://github.com/acme/billing-service/pull/142
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

func TestCheckRejectsDraftedMemoryRuleMissingCalibrationMetadata(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	rulesDir := filepath.Join(tempDir, "memory", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := `object_type: relia.memory_rule
schema_version: "1.0"
id: avoid-drafted-without-calibration
kind: avoid
status: active
statement: >
  Avoid retrying the generated schema snapshot path without checking error-shape assertions.
scope:
  paths:
    - packages/billing/
confidence: 0.8
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_001
provenance:
  - pr: 142
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/142
review:
  label: accepted
  statement_origin: cluster_summary
metadata:
  confidence_label: high
`
	if err := os.WriteFile(filepath.Join(rulesDir, "avoid-drafted-without-calibration.yaml"), []byte(rule), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "metadata.confidence_inputs.evidence_count") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestCheckAcceptsPlaybookRuleWithCleanMergeEvidence(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	rulesDir := filepath.Join(tempDir, "memory", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := `object_type: relia.memory_rule
schema_version: "1.0"
id: playbook-freeze-time
kind: playbook
status: active
statement: >
  Use the freeze-time fixture for billing rollover tests.
scope:
  paths:
    - packages/billing/
confidence: 0.8
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_clean_merge
provenance:
  - pr: 210
    outcome: merged_clean
    url: https://github.com/acme/billing-service/pull/210
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`
	if err := os.WriteFile(filepath.Join(rulesDir, "playbook-freeze-time.yaml"), []byte(rule), 0o644); err != nil {
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
  -
    pr: 141
    outcome: ci_failure
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
	if len(provenance) != 2 {
		t.Fatalf("provenance list maps = %#v", provenance)
	}
	if got := provenance[0]["pr"].Value; got != "141" {
		t.Fatalf("provenance pr = %q", got)
	}
	if got := provenance[0]["outcome"].Value; got != "ci_failure" {
		t.Fatalf("provenance outcome = %q", got)
	}
	if got := provenance[1]["pr"].Value; got != "142" {
		t.Fatalf("provenance pr = %q", got)
	}
	if got := provenance[1]["outcome"].Value; got != "revert" {
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
