package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	resultdoc "github.com/Clyra-AI/relia/internal/result"
)

func TestValidateRuleArtifactAcceptsActiveRule(t *testing.T) {
	root := t.TempDir()
	writeValidationFixture(t, root, validMemoryRuleYAML())

	if commandErr := ValidateRuleArtifact(root, filepath.Join(root, "memory", "rules", "rule.yaml"), testValidationOptions()); commandErr != nil {
		t.Fatalf("ValidateRuleArtifact returned error: %#v", commandErr)
	}
}

func TestValidateRuleArtifactAcceptsBlockScalarStatement(t *testing.T) {
	root := t.TempDir()
	content := strings.Replace(validMemoryRuleYAML(), "statement: Avoid repeating this failure.", "statement: >\n  Avoid repeating this failure.\n  Use the reviewed fixture.", 1)
	writeValidationFixture(t, root, content)

	if commandErr := ValidateRuleArtifact(root, filepath.Join(root, "memory", "rules", "rule.yaml"), testValidationOptions()); commandErr != nil {
		t.Fatalf("ValidateRuleArtifact returned error: %#v", commandErr)
	}
}

func TestValidateRuleArtifactRejectsEmptyBlockScalarStatement(t *testing.T) {
	root := t.TempDir()
	content := strings.Replace(validMemoryRuleYAML(), "statement: Avoid repeating this failure.", "statement: >\n", 1)
	writeValidationFixture(t, root, content)

	commandErr := ValidateRuleArtifact(root, filepath.Join(root, "memory", "rules", "rule.yaml"), testValidationOptions())
	if commandErr == nil {
		t.Fatal("expected validation error")
	}
	if commandErr.Message != "memory rule statement is required" {
		t.Fatalf("message = %q", commandErr.Message)
	}
}

func TestValidateRuleArtifactRejectsActiveRuleWithoutAcceptedReview(t *testing.T) {
	root := t.TempDir()
	content := strings.Replace(validMemoryRuleYAML(), "label: accepted", "label: suggested", 1)
	writeValidationFixture(t, root, content)

	commandErr := ValidateRuleArtifact(root, filepath.Join(root, "memory", "rules", "rule.yaml"), testValidationOptions())
	if commandErr == nil {
		t.Fatal("expected validation error")
	}
	if commandErr.Message != "active memory rule review.label must be accepted" {
		t.Fatalf("message = %q", commandErr.Message)
	}
}

func TestValidateRuleArtifactRejectsInvalidReviewGate(t *testing.T) {
	root := t.TempDir()
	content := strings.Replace(validMemoryRuleYAML(), "review:\n  label: accepted", "review:\n  gate: automatic\n  label: accepted", 1)
	writeValidationFixture(t, root, content)

	commandErr := ValidateRuleArtifact(root, filepath.Join(root, "memory", "rules", "rule.yaml"), testValidationOptions())
	if commandErr == nil {
		t.Fatal("expected validation error")
	}
	if commandErr.Message != "memory rule review.gate is invalid" {
		t.Fatalf("message = %q", commandErr.Message)
	}
}

func TestValidateRuleArtifactRejectsActiveRuleWithoutApprovedDecision(t *testing.T) {
	root := t.TempDir()
	content := strings.Replace(validMemoryRuleYAML(), "review:\n  label: accepted", "review:\n  label: accepted\n  decision: pending", 1)
	writeValidationFixture(t, root, content)

	commandErr := ValidateRuleArtifact(root, filepath.Join(root, "memory", "rules", "rule.yaml"), testValidationOptions())
	if commandErr == nil {
		t.Fatal("expected validation error")
	}
	if commandErr.Message != "active memory rule review.decision must be approved" {
		t.Fatalf("message = %q", commandErr.Message)
	}
}

func writeValidationFixture(t *testing.T, root string, content string) {
	t.Helper()
	sourcePath := filepath.Join(root, "cmd", "relia")
	if err := os.MkdirAll(sourcePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ruleDir := filepath.Join(root, "memory", "rules")
	if err := os.MkdirAll(ruleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ruleDir, "rule.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testValidationOptions() ValidationOptions {
	return ValidationOptions{
		SchemaVersion: "1.0",
		ArtifactContractError: func(message string, ref string) *resultdoc.CommandError {
			return &resultdoc.CommandError{Type: "artifact_contract_validation_failed", Message: message, Ref: ref}
		},
		InternalError: func(message string, err error) *resultdoc.CommandError {
			return &resultdoc.CommandError{Type: "internal", Message: message}
		},
		RepoPathExists: func(root string, rel string) bool {
			_, err := os.Stat(filepath.Join(root, rel))
			return err == nil
		},
	}
}

func validMemoryRuleYAML() string {
	return `object_type: relia.memory_rule
schema_version: "1.0"
id: rule-1
kind: avoid
status: active
statement: Avoid repeating this failure.
scope:
  paths:
    - cmd/relia/main.go
confidence: 0.8
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp-1
provenance:
  - url: https://github.com/Clyra-AI/relia/pull/1
    pr: 1
    outcome: review_correction
review:
  label: accepted
  statement_origin: cluster_summary
metadata:
  confidence_label: high
  confidence_inputs:
    evidence_count: 1
    contradictions: 0
    recency_weight: 1
    flake_discount: 0
    extraction_confidence: 1
    drafting_model_weight: 0
  decay:
    half_life_days: 90
    latest_evidence_at: "2026-07-01T00:00:00Z"
    oldest_evidence_at: "2026-07-01T00:00:00Z"
    anchor_recorded_at: "2026-07-01T00:00:00Z"
`
}
