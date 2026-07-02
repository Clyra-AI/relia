package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDemoAssessMatchesDirectoryScopedActiveRule(t *testing.T) {
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

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "change.diff"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	assessment := decodeAssessmentFromResult(t, decodeResult(t, stdout))
	if assessment.RiskLevel != "match_high" {
		t.Fatalf("risk_level = %q, want match_high", assessment.RiskLevel)
	}
	if fmt.Sprint(assessment.Matches) != fmt.Sprint([]demoAssessmentExpectedRule{{RuleID: "billing-time-fixture", Confidence: 0.86}}) {
		t.Fatalf("matches = %#v", assessment.Matches)
	}
	if fmt.Sprint(assessment.Citations) != fmt.Sprint([]string{"https://github.com/acme/billing-service/pull/142"}) {
		t.Fatalf("citations = %#v", assessment.Citations)
	}
}

func TestServeExposesLocalMCPCapabilityManifestForActiveRules(t *testing.T) {
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
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "candidate.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: candidate-rule
kind: avoid
status: candidate
statement: Candidate rules must not be served.
scope:
  paths:
    - packages/billing/
confidence: 0.86
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0143
provenance:
  - pr: 143
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/143
review:
  label: suggested
  statement_origin: cluster_summary
metadata: {}
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "serve", "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("serve exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "serve" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	if result.Data["hosted_service_required"] != false || result.Data["live_network_required"] != false {
		t.Fatalf("serve boundary = %#v", result.Data)
	}
	mcp := result.Data["mcp"].(map[string]any)
	tools := stringsFromInterfaceSlice(t, mcp["tools"])
	if fmt.Sprint(tools) != fmt.Sprint([]string{"recall", "assess", "coverage"}) {
		t.Fatalf("tools = %#v", tools)
	}
	if got := int(result.Data["active_rule_count"].(float64)); got != 1 {
		t.Fatalf("active_rule_count = %d", got)
	}
	rules := result.Data["served_rules"].([]any)
	if len(rules) != 1 || !strings.Contains(fmt.Sprint(rules[0]), "billing-time-fixture") || strings.Contains(fmt.Sprint(rules[0]), "candidate-rule") {
		t.Fatalf("served_rules = %#v", rules)
	}
}

func TestServeFiltersPlaybookCitationsToCleanEvidence(t *testing.T) {
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
  count: 2
  contradictions: 1
  experiences:
    - exp_0141
    - exp_0142
provenance:
  - pr: 141
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/141
  - pr: 142
    outcome: merged_clean
    url: https://github.com/acme/billing-service/pull/142
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "serve", "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("serve exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	rules := result.Data["served_rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("served_rules = %#v", rules)
	}
	rule := rules[0].(map[string]any)
	citations := stringsFromInterfaceSlice(t, rule["citations"])
	if fmt.Sprint(citations) != fmt.Sprint([]string{"https://github.com/acme/billing-service/pull/142"}) {
		t.Fatalf("citations = %#v", citations)
	}
}

func TestServeFailsClosedForInvalidRuleCitationURL(t *testing.T) {
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
    url: https://example.com/not-a-pr
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "serve", "--format", "json"}, false)

	if code == ExitSuccess {
		t.Fatalf("serve should fail closed for invalid citation URL, stderr = %q, stdout = %q", stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Message, "citation URL") {
		t.Fatalf("serve error = %#v", result.Errors)
	}
}

func TestDemoAssessReportsCoveredCleanForAcceptedPlaybookRule(t *testing.T) {
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

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "change.diff"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	assessment := decodeAssessmentFromResult(t, decodeResult(t, stdout))
	if assessment.RiskLevel != "covered_clean" {
		t.Fatalf("risk_level = %q, want covered_clean", assessment.RiskLevel)
	}
	if fmt.Sprint(assessment.Matches) != fmt.Sprint([]demoAssessmentExpectedRule{{RuleID: "billing-playbook-fixture", Confidence: 0.91}}) {
		t.Fatalf("matches = %#v", assessment.Matches)
	}
}

func TestDemoAssessIgnoresUnservedPlaybookFailureCitationShape(t *testing.T) {
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
  count: 2
  contradictions: 0
  experiences:
    - exp_0142
    - exp_0143
provenance:
  - pr: 142
    outcome: merged_clean
    url: https://github.com/acme/billing-service/pull/142
  - pr: 143
    outcome: ci_failure
    url: https://example.com/not-a-pull-request
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

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "change.diff"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	assessment := decodeAssessmentFromResult(t, decodeResult(t, stdout))
	if assessment.RiskLevel != "covered_clean" {
		t.Fatalf("risk_level = %q, want covered_clean", assessment.RiskLevel)
	}
	if fmt.Sprint(assessment.Citations) != fmt.Sprint([]string{"https://github.com/acme/billing-service/pull/142"}) {
		t.Fatalf("citations = %#v", assessment.Citations)
	}
}

func TestDemoAssessRejectsPlaybookRuleWithoutPositiveEvidence(t *testing.T) {
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
+    return billing_clock().strftime("%Y-%m-%d")
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "change.diff"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "fix_held or merged_clean") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsPlaybookRuleWithoutPositiveCitation(t *testing.T) {
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
  count: 2
  contradictions: 0
  experiences:
    - exp_0142
provenance:
  - pr: 142
    outcome: merged_clean
  - pr: 143
    outcome: ci_failure
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
+    return billing_clock().strftime("%Y-%m-%d")
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "change.diff"}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "citation URLs") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessMatchesExistingDirectoryScopeWithoutTrailingSlash(t *testing.T) {
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
    - packages/billing
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

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "change.diff"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	assessment := decodeAssessmentFromResult(t, decodeResult(t, stdout))
	if assessment.RiskLevel != "match_high" {
		t.Fatalf("risk_level = %q, want match_high", assessment.RiskLevel)
	}
	if fmt.Sprint(assessment.Matches) != fmt.Sprint([]demoAssessmentExpectedRule{{RuleID: "billing-time-fixture", Confidence: 0.86}}) {
		t.Fatalf("matches = %#v", assessment.Matches)
	}
}

func TestDemoAssessMatchesHistoricalDirectoryScopeWithoutTrailingSlash(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), `def rollover_day():
    return "2026-01-01"
`)
	runGitForTest(t, tempDir, "init")
	runGitForTest(t, tempDir, "config", "user.email", "relia-test@example.com")
	runGitForTest(t, tempDir, "config", "user.name", "Relia Test")
	runGitForTest(t, tempDir, "add", "packages/billing/invoice.py")
	runGitForTest(t, tempDir, "commit", "-m", "add historical billing package")
	if err := os.RemoveAll(filepath.Join(tempDir, "packages", "billing")); err != nil {
		t.Fatal(err)
	}
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "billing-time.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-time-fixture
kind: avoid
status: active
statement: Use the billing clock fixture instead of direct UTC calls.
scope:
  paths:
    - packages/billing
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

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "change.diff"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	assessment := decodeAssessmentFromResult(t, decodeResult(t, stdout))
	if assessment.RiskLevel != "match_high" {
		t.Fatalf("risk_level = %q, want match_high", assessment.RiskLevel)
	}
	if fmt.Sprint(assessment.Matches) != fmt.Sprint([]demoAssessmentExpectedRule{{RuleID: "billing-time-fixture", Confidence: 0.86}}) {
		t.Fatalf("matches = %#v", assessment.Matches)
	}
}

func TestAssessFormatJSONOverridesInteractiveOutput(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "unknown.diff"), `diff --git a/packages/search/query.py b/packages/search/query.py
--- a/packages/search/query.py
+++ b/packages/search/query.py
@@ -1,2 +1,3 @@
 def normalize_query(value):
-    return value.strip().lower()
+    normalized = value.strip().lower()
+    return " ".join(normalized.split())
`)

	stdout, stderr, code := runForTest(t, []string{"assess", "--input", "unknown.diff", "--format", "json"}, true)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "assess" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	assessment := decodeAssessmentFromResult(t, result)
	if assessment.RiskLevel != "no_coverage" {
		t.Fatalf("risk_level = %q, want no_coverage", assessment.RiskLevel)
	}
}

func TestAssessDefaultFormatShowsAssessmentInInteractiveOutput(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "unknown.diff"), `diff --git a/packages/search/query.py b/packages/search/query.py
--- a/packages/search/query.py
+++ b/packages/search/query.py
@@ -1,2 +1,3 @@
 def normalize_query(value):
-    return value.strip().lower()
+    normalized = value.strip().lower()
+    return " ".join(normalized.split())
`)

	stdout, stderr, code := runForTest(t, []string{"assess", "--input", "unknown.diff"}, true)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	assessment := decodeAssessmentFromResult(t, result)
	if assessment.RiskLevel != "no_coverage" {
		t.Fatalf("risk_level = %q, want no_coverage", assessment.RiskLevel)
	}
}
