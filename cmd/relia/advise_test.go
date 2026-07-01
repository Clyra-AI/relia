package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestAdviseClearsExistingAdvisoryForCoveredCleanAssessment(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "billing-playbook.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-playbook-fixture
kind: playbook
status: active
statement: Billing clock changes are covered when they use the approved fixture.
scope:
  paths:
    - packages/billing/
confidence: 0.91
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0142
provenance:
  - pr: 142
    outcome: merged_clean
    url: https://github.com/acme/billing-service/pull/142
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`)
	diffContent := `diff --git a/packages/billing/invoice.py b/packages/billing/invoice.py
--- a/packages/billing/invoice.py
+++ b/packages/billing/invoice.py
@@ -1,2 +1,3 @@
 def rollover_day():
-    return "2026-01-01"
+    return billing_clock().strftime("%Y-%m-%d")
`
	writeFileForTest(t, filepath.Join(tempDir, "change.diff"), diffContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "reports", "advisory-state.json"), fmt.Sprintf(`{
  "object_type": "relia.advisory_state",
  "schema_version": "1.0",
  "diff_fingerprint": %q,
  "metadata": {
    "risk_level": "match_high"
  }
}
`, sha256String(diffContent)))

	stdout, stderr, code := runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("advise covered clean clear exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Data["should_comment"] != true || result.Data["skip_reason"] != "" {
		t.Fatalf("advise result = %#v", result.Data)
	}
	comment, err := os.ReadFile(filepath.Join(tempDir, ".relia", "reports", "advisory-comment.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Prior advisory cleared", "billing-playbook-fixture", "https://github.com/acme/billing-service/pull/142"} {
		if !strings.Contains(string(comment), want) {
			t.Fatalf("clear comment missing %q:\n%s", want, comment)
		}
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("second advise covered clean exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result = decodeResult(t, stdout)
	if result.Data["should_comment"] != false || result.Data["skip_reason"] != "unchanged_diff_fingerprint" {
		t.Fatalf("second advise result = %#v", result.Data)
	}
}

func TestAdviseClearsExistingAdvisoryWhenConfidenceDropsBelowMinimum(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "billing-low-confidence.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-low-confidence-fixture
kind: avoid
status: active
statement: Low confidence billing changes should not keep stale advisories visible.
scope:
  paths:
    - packages/billing/
confidence: 0.55
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
	diffContent := `diff --git a/packages/billing/invoice.py b/packages/billing/invoice.py
--- a/packages/billing/invoice.py
+++ b/packages/billing/invoice.py
@@ -1,2 +1,3 @@
 def rollover_day():
-    return "2026-01-01"
+    return maybe_safe_billing_clock().strftime("%Y-%m-%d")
`
	writeFileForTest(t, filepath.Join(tempDir, "change.diff"), diffContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "reports", "advisory-state.json"), fmt.Sprintf(`{
  "object_type": "relia.advisory_state",
  "schema_version": "1.0",
  "diff_fingerprint": "sha256:previous",
  "metadata": {
    "generated_at": %q,
    "risk_level": "match_high"
  }
}
`, time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)))

	stdout, stderr, code := runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("advise below-min clear exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Data["should_comment"] != true || result.Data["skip_reason"] != "below_min_confidence" {
		t.Fatalf("advise result = %#v", result.Data)
	}
	comment, err := os.ReadFile(filepath.Join(tempDir, ".relia", "reports", "advisory-comment.md"))
	if err != nil {
		t.Fatal(err)
	}
	commentText := string(comment)
	for _, want := range []string{"risk_level=below_min_confidence", "below the advisory confidence threshold", "Prior advisory cleared", "billing-low-confidence-fixture"} {
		if !strings.Contains(commentText, want) {
			t.Fatalf("below-min clear comment missing %q:\n%s", want, commentText)
		}
	}
	stateContent, err := os.ReadFile(filepath.Join(tempDir, ".relia", "reports", "advisory-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(stateContent, &state); err != nil {
		t.Fatalf("decode advisory state: %v\n%s", err, stateContent)
	}
	metadata := state["metadata"].(map[string]any)
	if metadata["risk_level"] != "below_min_confidence" {
		t.Fatalf("state metadata risk_level = %#v, state = %#v", metadata["risk_level"], state)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("second advise below-min exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result = decodeResult(t, stdout)
	if result.Data["should_comment"] != false || result.Data["skip_reason"] != "unchanged_diff_fingerprint" {
		t.Fatalf("second advise result = %#v", result.Data)
	}
}

func TestAdviseIgnoresPlaybookConfidenceForCommentThreshold(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "billing-low-confidence.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-low-confidence-fixture
kind: avoid
status: active
statement: Low confidence billing changes should not publish advisory comments.
scope:
  paths:
    - packages/billing/
confidence: 0.55
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
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "billing-playbook.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-playbook-fixture
kind: playbook
status: active
statement: Billing clock changes are covered when they use the approved fixture.
scope:
  paths:
    - packages/billing/
confidence: 0.91
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0143
provenance:
  - pr: 143
    outcome: merged_clean
    url: https://github.com/acme/billing-service/pull/143
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
+    return maybe_safe_billing_clock().strftime("%Y-%m-%d")
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("advise mixed confidence exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Data["should_comment"] != false || result.Data["skip_reason"] != "below_min_confidence" {
		t.Fatalf("advise result = %#v", result.Data)
	}
	assessment := result.Data["assessment"].(map[string]any)
	metadata := assessment["metadata"].(map[string]any)
	if metadata["max_avoid_confidence"] != float64(0.55) {
		t.Fatalf("max_avoid_confidence = %#v, metadata = %#v", metadata["max_avoid_confidence"], metadata)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "reports", "advisory-comment.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("advisory comment should not be written for low-confidence avoid match, stat err = %v", err)
	}
}

func TestAdviseRewarnsUnchangedDiffWhenPriorMarkerWasClear(t *testing.T) {
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
	diffContent := `diff --git a/packages/billing/invoice.py b/packages/billing/invoice.py
--- a/packages/billing/invoice.py
+++ b/packages/billing/invoice.py
@@ -1,2 +1,3 @@
 def rollover_day():
-    return "2026-01-01"
+    return datetime.utcnow().strftime("%Y-%m-%d")
`
	writeFileForTest(t, filepath.Join(tempDir, "change.diff"), diffContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "reports", "advisory-state.json"), fmt.Sprintf(`{
  "object_type": "relia.advisory_state",
  "schema_version": "1.0",
  "diff_fingerprint": %q,
  "metadata": {
    "generated_at": %q,
    "risk_level": "covered_clean"
  }
}
`, sha256String(diffContent), time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)))

	stdout, stderr, code := runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("advise rewarn exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Data["should_comment"] != true || result.Data["skip_reason"] != "" {
		t.Fatalf("advise result = %#v", result.Data)
	}
	comment, err := os.ReadFile(filepath.Join(tempDir, ".relia", "reports", "advisory-comment.md"))
	if err != nil {
		t.Fatal(err)
	}
	commentText := string(comment)
	for _, want := range []string{"risk_level=match_high", "billing-time-fixture", "https://github.com/acme/billing-service/pull/142"} {
		if !strings.Contains(commentText, want) {
			t.Fatalf("rewarn comment missing %q:\n%s", want, commentText)
		}
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("second advise rewarn exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result = decodeResult(t, stdout)
	if result.Data["should_comment"] != false || result.Data["skip_reason"] != "unchanged_diff_fingerprint" {
		t.Fatalf("second advise result = %#v", result.Data)
	}
}

func TestAdviseStaysSilentForCoveredCleanAssessment(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "billing-playbook.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-playbook-fixture
kind: playbook
status: active
statement: Billing clock changes are covered when they use the approved fixture.
scope:
  paths:
    - packages/billing/
confidence: 0.91
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0142
provenance:
  - pr: 142
    outcome: merged_clean
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
+    return billing_clock().strftime("%Y-%m-%d")
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("advise covered clean exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Data["should_comment"] != false || result.Data["skip_reason"] != "covered_clean" {
		t.Fatalf("advise result = %#v", result.Data)
	}
}

func TestAdviseDebouncesChangedDiffWhenPriorMarkerIsFresh(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	priorGeneratedAt := time.Now().UTC().Format(time.RFC3339)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "reports", "advisory-state.json"), fmt.Sprintf(`{
  "object_type": "relia.advisory_state",
  "schema_version": "1.0",
  "diff_fingerprint": "sha256:previous",
  "metadata": {
    "generated_at": %q,
    "source": "existing_github_comment_marker"
  }
}
`, priorGeneratedAt))
	writeFileForTest(t, filepath.Join(tempDir, "change.diff"), `diff --git a/packages/new/path.py b/packages/new/path.py
--- a/packages/new/path.py
+++ b/packages/new/path.py
@@ -1,2 +1,3 @@
 def work():
-    return "old"
+    return "new"
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "advise", "--input", "change.diff", "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("advise debounce exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Data["should_comment"] != false || result.Data["skip_reason"] != "reassess_debounce_window" {
		t.Fatalf("advise result = %#v", result.Data)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "reports", "advisory-comment.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("advisory comment should not be written inside debounce window, stat err = %v", err)
	}
	stateContent, err := os.ReadFile(filepath.Join(tempDir, ".relia", "reports", "advisory-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(stateContent, &state); err != nil {
		t.Fatalf("decode advisory state: %v\n%s", err, stateContent)
	}
	if state["diff_fingerprint"] != "sha256:previous" {
		t.Fatalf("debounce state diff_fingerprint = %#v, want prior fingerprint; state = %#v", state["diff_fingerprint"], state)
	}
	metadata := state["metadata"].(map[string]any)
	if metadata["generated_at"] != priorGeneratedAt {
		t.Fatalf("debounce state generated_at = %#v, want prior generated_at %q", metadata["generated_at"], priorGeneratedAt)
	}
	if metadata["debounced_diff_fingerprint"] != result.Data["diff_fingerprint"] {
		t.Fatalf("debounced_diff_fingerprint = %#v, want current diff %q", metadata["debounced_diff_fingerprint"], result.Data["diff_fingerprint"])
	}
}

func TestAdvisoryWorkflowKeepsTokenOutOfReliaBuildAndSeedsState(t *testing.T) {
	sourceRoot := findRepoRootForTest(t)
	contentBytes, err := os.ReadFile(filepath.Join(sourceRoot, ".github", "workflows", "relia-advisory.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(contentBytes)
	if !strings.Contains(content, "\n  pull_request_target:") || strings.Contains(content, "\n  pull_request:") {
		t.Fatalf("advisory workflow must use pull_request_target with trusted base checkout:\n%s", content)
	}
	prepareBlock := workflowStepBlock(content, "Prepare advisory inputs")
	if !strings.Contains(prepareBlock, "GH_TOKEN: ${{ github.token }}") {
		t.Fatalf("prepare step must own GitHub token for API reads:\n%s", prepareBlock)
	}
	checkoutBlock := workflowStepBlock(content, "Check out repository")
	if !strings.Contains(checkoutBlock, "ref: ${{ github.event.pull_request.base.sha }}") {
		t.Fatalf("checkout must run trusted base-revision Relia code and rules:\n%s", checkoutBlock)
	}
	if !strings.Contains(checkoutBlock, "persist-credentials: false") {
		t.Fatalf("checkout must not persist the GitHub token for later PR-code execution:\n%s", checkoutBlock)
	}
	for _, want := range []string{".relia/reports/github-comments.json", "advisory-state.json", "diff_fingerprint", "generated_at", "risk_level"} {
		if !strings.Contains(prepareBlock, want) {
			t.Fatalf("prepare step missing %q:\n%s", want, prepareBlock)
		}
	}
	for _, want := range []string{"gh api --paginate --slurp", "flatten_comments"} {
		if !strings.Contains(prepareBlock, want) {
			t.Fatalf("prepare step must paginate and flatten advisory comments; missing %q:\n%s", want, prepareBlock)
		}
	}
	for _, want := range []string{"trusted_marker_authors", "github-actions[bot]", ".get(\"user\")"} {
		if !strings.Contains(prepareBlock, want) {
			t.Fatalf("prepare step must trust only bot-authored advisory markers; missing %q:\n%s", want, prepareBlock)
		}
	}
	assertWorkflowPythonHeredocParses(t, prepareBlock, "prepare")
	buildBlock := workflowStepBlock(content, "Build PR advisory")
	if strings.Contains(buildBlock, "GH_TOKEN") || strings.Contains(buildBlock, "github.token") {
		t.Fatalf("build step must run relia without a GitHub write token:\n%s", buildBlock)
	}
	if !strings.Contains(buildBlock, "go run ./cmd/relia --json advise") {
		t.Fatalf("build step missing relia advise invocation:\n%s", buildBlock)
	}
	for _, want := range []string{"trusted_base_advisory_unavailable", "unknown command"} {
		if !strings.Contains(buildBlock, want) {
			t.Fatalf("build step must skip advisory-only publish when trusted base lacks advise; missing %q:\n%s", want, buildBlock)
		}
	}
	assertWorkflowPythonHeredocParses(t, buildBlock, "build")
	publishBlock := workflowStepBlock(content, "Publish one advisory comment")
	for _, forbidden := range []string{"advisory-env", ". .relia/reports", "source .relia/reports"} {
		if strings.Contains(publishBlock, forbidden) {
			t.Fatalf("publish step must not source PR-controlled shell env; found %q:\n%s", forbidden, publishBlock)
		}
	}
	for _, want := range []string{"advisory-result.json", "flatten_comments", "subprocess.run(command", "advisory-body.json"} {
		if !strings.Contains(publishBlock, want) {
			t.Fatalf("publish step must parse advisory JSON and invoke gh without shell; missing %q:\n%s", want, publishBlock)
		}
	}
	for _, want := range []string{"trusted_marker_authors", "github-actions[bot]", ".get(\"user\")"} {
		if !strings.Contains(publishBlock, want) {
			t.Fatalf("publish step must update only bot-authored advisory markers; missing %q:\n%s", want, publishBlock)
		}
	}
	if strings.Contains(publishBlock, "\n          else\n") || !strings.Contains(publishBlock, "\n          else:") {
		t.Fatalf("publish step Python branch must use valid else syntax:\n%s", publishBlock)
	}
	assertWorkflowPythonHeredocParses(t, publishBlock, "publish")
}

func assertWorkflowPythonHeredocParses(t *testing.T, block string, label string) {
	t.Helper()
	publishScript := workflowPythonHeredocForTest(t, block)
	cmd := exec.Command("python3", "-c", "import ast, sys; ast.parse(sys.stdin.read())")
	cmd.Stdin = strings.NewReader(publishScript)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s step Python heredoc must parse, err = %v, output = %s\n%s", label, err, output, publishScript)
	}
}

func workflowStepBlock(content string, name string) string {
	marker := "- name: " + name
	start := strings.Index(content, marker)
	if start < 0 {
		return ""
	}
	rest := content[start+len(marker):]
	next := strings.Index(rest, "\n      - name: ")
	if next < 0 {
		return content[start:]
	}
	return content[start : start+len(marker)+next]
}

func workflowPythonHeredocForTest(t *testing.T, block string) string {
	t.Helper()
	marker := "python3 - <<'PY'"
	start := strings.Index(block, marker)
	if start < 0 {
		t.Fatalf("workflow step missing Python heredoc:\n%s", block)
	}
	lines := strings.Split(block[start+len(marker):], "\n")
	var script []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "PY" {
			if len(script) == 0 {
				t.Fatalf("workflow Python heredoc is empty:\n%s", block)
			}
			return strings.Join(script, "\n") + "\n"
		}
		script = append(script, strings.TrimPrefix(line, "          "))
	}
	t.Fatalf("workflow Python heredoc is unterminated:\n%s", block)
	return ""
}
