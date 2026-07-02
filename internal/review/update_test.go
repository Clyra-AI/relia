package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Clyra-AI/relia/internal/yamlmini"
)

func TestFindRulePathByID(t *testing.T) {
	root := setupReviewRuleRepo(t, "candidate")

	path, commandErr := FindRulePath(root, "memory/rules", "avoid-demo", testUpdateOptions())
	if commandErr != nil {
		t.Fatalf("FindRulePath returned error: %v", commandErr)
	}
	if got, want := filepath.ToSlash(path), "memory/rules/avoid-demo.yaml"; !strings.HasSuffix(got, want) {
		t.Fatalf("path = %q, want suffix %q", got, want)
	}
}

func TestUpdateRuleReviewApproveActivatesRule(t *testing.T) {
	root := setupReviewRuleRepo(t, "candidate")
	rulePath := filepath.Join(root, "memory", "rules", "avoid-demo.yaml")

	status, commandErr := UpdateRuleReview(root, rulePath, Options{
		Action: "approve",
		Label:  "accepted",
		Rule:   "avoid-demo",
	}, testUpdateOptions())
	if commandErr != nil {
		t.Fatalf("UpdateRuleReview returned error: %v", commandErr)
	}
	if status != "active" {
		t.Fatalf("status = %q, want active", status)
	}
	document := readReviewRuleDocument(t, rulePath)
	if got := document.Scalars["status"].Value; got != "active" {
		t.Fatalf("status scalar = %q, want active", got)
	}
	if got := document.Scalars["review.label"].Value; got != "accepted" {
		t.Fatalf("review.label = %q, want accepted", got)
	}
	if got := document.Scalars["metadata.lifecycle_reason"].Value; got != "approved by human review" {
		t.Fatalf("metadata.lifecycle_reason = %q", got)
	}
}

func TestUpdateRuleReviewEditUpdatesStatementAndScope(t *testing.T) {
	root := setupReviewRuleRepo(t, "candidate")
	rulePath := filepath.Join(root, "memory", "rules", "avoid-demo.yaml")
	mustWriteFile(t, filepath.Join(root, "internal", "review", "update.go"), []byte("package review\n"))

	status, commandErr := UpdateRuleReview(root, rulePath, Options{
		Action:     "edit",
		Label:      "suggested",
		Rule:       "avoid-demo",
		Statement:  "Prefer build: lint before review.",
		ScopePaths: []string{"internal/review/update.go", "cmd/relia/main.go"},
	}, testUpdateOptions())
	if commandErr != nil {
		t.Fatalf("UpdateRuleReview returned error: %v", commandErr)
	}
	if status != "candidate" {
		t.Fatalf("status = %q, want candidate", status)
	}
	content, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `statement: "Prefer build: lint before review."`) {
		t.Fatalf("statement was not YAML-quoted:\n%s", string(content))
	}
	document := readReviewRuleDocument(t, rulePath)
	if got := document.Scalars["review.statement_origin"].Value; got != "human_authored" {
		t.Fatalf("review.statement_origin = %q, want human_authored", got)
	}
	if got := document.Lists["scope.paths"]; len(got) != 2 || got[0].Value != "cmd/relia/main.go" || got[1].Value != "internal/review/update.go" {
		t.Fatalf("scope.paths = %#v", got)
	}
	if got := document.Scalars["metadata.lifecycle_reason"].Value; got != "edited by human review; pending approval" {
		t.Fatalf("metadata.lifecycle_reason = %q", got)
	}
}

func TestUpdateRuleReviewRejectsRetiredApproval(t *testing.T) {
	root := setupReviewRuleRepo(t, "retired")
	rulePath := filepath.Join(root, "memory", "rules", "avoid-demo.yaml")

	_, commandErr := UpdateRuleReview(root, rulePath, Options{
		Action: "approve",
		Label:  "accepted",
		Rule:   "avoid-demo",
	}, testUpdateOptions())
	if commandErr == nil {
		t.Fatal("expected retired approval error")
	}
	if commandErr.Message != "cannot mark retired memory rule accepted without fresh distill evidence" {
		t.Fatalf("message = %q", commandErr.Message)
	}
}

func setupReviewRuleRepo(t *testing.T, status string) string {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "cmd", "relia", "main.go"), []byte("package main\n"))
	label := "suggested"
	if status == "active" {
		label = "accepted"
	}
	mustWriteFile(t, filepath.Join(root, "memory", "rules", "avoid-demo.yaml"), []byte(reviewRuleYAML(status, label)))
	return root
}

func reviewRuleYAML(status string, label string) string {
	return `object_type: relia.memory_rule
schema_version: "1.0"
id: avoid-demo
kind: avoid
status: ` + status + `
statement: Avoid repeating the demo failure.
scope:
  paths:
    - cmd/relia/main.go
  signals:
confidence: 0.8
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp-1
provenance:
  - pr: 12
    outcome: ci_failure
    url: https://github.com/Clyra-AI/relia/pull/12
    experience_id: exp-1
review:
  label: ` + label + `
  statement_origin: cluster_summary
metadata:
  confidence_label: high
  lifecycle_reason: drafted from recurrence
  confidence_inputs:
    evidence_count: 1
    recency_weight: 1
    contradictions: 0
    flake_discount: 0
    extraction_confidence: 1
    drafting_model_weight: 0
  decay:
    half_life_days: 90
    latest_evidence_at: 2026-04-10T10:00:00Z
    oldest_evidence_at: 2026-04-10T10:00:00Z
    anchor_recorded_at: 2026-04-10T10:00:00Z
  cluster:
    key: demo-key
    key_hash: demo-hash
    provenance: signature
  source_artifact_digest: sha256:demo
  source_artifacts:
    - .relia/experiences/2026-04.jsonl
  provider: signature
  embedding_mode: signature
  review_required: true
  deterministic_fallback: true
  memory_source: verified_outcome_events
  source_record_type: verified_outcome_event
  excluded_memory_sources:
    - agent_self_report
  generated_by: relia distill
  redaction_status: applied
`
}

func readReviewRuleDocument(t *testing.T, path string) yamlmini.Document {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document, err := yamlmini.ParseDocument(string(content))
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func testUpdateOptions() UpdateOptions {
	return UpdateOptions{SchemaVersion: "1.0"}
}

func mustWriteFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}
