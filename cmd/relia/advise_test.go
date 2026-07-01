package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdviseWritesOneAdvisoryCommentPlanAndSkipsUnchangedDiff(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "billing-time.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-time-fixture
kind: avoid
status: active
statement: Use the billing clock fixture instead of direct UTC calls.
scope:
  paths:
    - packages/billing/
confidence: 0.86
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0142
provenance:
  - pr: 142
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/142
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`)
	writeFileForTest(t, filepath.Join(tempDir, "change.diff"), `diff --git a/packages/billing/invoice.py b/packages/billing/invoice.py
--- a/packages/billing/invoice.py
+++ b/packages/billing/invoice.py
@@ -1,2 +1,3 @@
 def rollover_day():
-    return "2026-01-01"
+    return datetime.utcnow().strftime("%Y-%m-%d")
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("advise exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Data["should_comment"] != true {
		t.Fatalf("advise result = %#v", result.Data)
	}
	if result.Data["comment_strategy"].(map[string]any)["max_comments_per_pr"].(float64) != 1 {
		t.Fatalf("comment_strategy = %#v", result.Data["comment_strategy"])
	}
	commentPath := filepath.Join(tempDir, ".relia", "reports", "advisory-comment.md")
	comment, err := os.ReadFile(commentPath)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(comment), "<!-- relia-advisory:v1"); count != 1 {
		t.Fatalf("comment marker count = %d, want 1:\n%s", count, comment)
	}
	for _, want := range []string{"<!-- relia-advisory:v1", "Relia advisory", "billing-time-fixture", "https://github.com/acme/billing-service/pull/142"} {
		if !strings.Contains(string(comment), want) {
			t.Fatalf("comment missing %q:\n%s", want, comment)
		}
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("second advise exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result = decodeResult(t, stdout)
	if result.Data["should_comment"] != false || result.Data["skip_reason"] != "unchanged_diff_fingerprint" {
		t.Fatalf("second advise result = %#v", result.Data)
	}
}

func TestAdviseHonorsZeroCommentCap(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "  max_comments_per_pr: 1", "  max_comments_per_pr: 0")
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "billing-time.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-time-fixture
kind: avoid
status: active
statement: Use the billing clock fixture instead of direct UTC calls.
scope:
  paths:
    - packages/billing/
confidence: 0.86
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0142
provenance:
  - pr: 142
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/142
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`)
	writeFileForTest(t, filepath.Join(tempDir, "change.diff"), `diff --git a/packages/billing/invoice.py b/packages/billing/invoice.py
--- a/packages/billing/invoice.py
+++ b/packages/billing/invoice.py
@@ -1,2 +1,3 @@
 def rollover_day():
-    return "2026-01-01"
+    return datetime.utcnow().strftime("%Y-%m-%d")
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("advise exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Data["should_comment"] != false || result.Data["skip_reason"] != "comment_cap_zero" {
		t.Fatalf("advise result = %#v", result.Data)
	}
	if result.Data["comment_strategy"].(map[string]any)["max_comments_per_pr"].(float64) != 0 {
		t.Fatalf("comment_strategy = %#v", result.Data["comment_strategy"])
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "reports", "advisory-comment.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("advisory comment should not be written when max_comments_per_pr is 0, stat err = %v", err)
	}
}

func TestAdviseEscapesTouchedPathsInAdvisoryComment(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	diffContent := "diff --git a/packages/weird/`@here.py b/packages/weird/`@here.py\n" +
		"--- a/packages/weird/`@here.py\n" +
		"+++ b/packages/weird/`@here.py\n" +
		"@@ -1,2 +1,3 @@\n" +
		" def work():\n" +
		"-    return \"old\"\n" +
		"+    return \"new\"\n"
	writeFileForTest(t, filepath.Join(tempDir, "change.diff"), diffContent)

	stdout, stderr, code := runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("advise exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	assessment := result.Data["assessment"].(map[string]any)
	if result.Data["should_comment"] != true || assessment["risk_level"] != "no_coverage" {
		t.Fatalf("advise result = %#v", result.Data)
	}
	comment, err := os.ReadFile(filepath.Join(tempDir, ".relia", "reports", "advisory-comment.md"))
	if err != nil {
		t.Fatal(err)
	}
	commentText := string(comment)
	if strings.Contains(commentText, "`packages/weird/`@here.py`") {
		t.Fatalf("comment rendered unsafe single-backtick path span:\n%s", commentText)
	}
	if !strings.Contains(commentText, "`` packages/weird/`@here.py ``") {
		t.Fatalf("comment missing escaped path code span:\n%s", commentText)
	}
}
