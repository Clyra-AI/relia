package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIngestRejectsMalformedNumericFieldsBeforePersistence(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		extraField string
		wantError  string
	}{
		{
			name:       "flake_discount",
			extraField: `"flake_discount": "0.5x",`,
			wantError:  "flake_discount must be numeric",
		},
		{
			name:       "attribution_confidence",
			extraField: `"attribution_confidence": "high",`,
			wantError:  "attribution confidence must be numeric",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tempDir := setupContractRepo(t)
			t.Chdir(tempDir)
			inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
			writeFileForTest(t, inputPath, `[
  {
    "experience_id": "exp_0143_numeric",
    "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
    "recorded_at": "2026-04-03T18:21:00Z",
    "pr": 143,
    "commit": "def5678",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    `+testCase.extraField+`
    "outcome_kind": "ci_failure",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/143"]
  }
]`)

			stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

			if code != ExitValidation {
				t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if result.Errors[0].Type != "artifact_contract_validation_failed" {
				t.Fatalf("error type = %q", result.Errors[0].Type)
			}
			if !strings.Contains(result.Errors[0].Message, testCase.wantError) {
				t.Fatalf("error message = %q, want %q", result.Errors[0].Message, testCase.wantError)
			}
			if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("experience shard should not be persisted on malformed numeric field: %v", err)
			}
		})
	}
}

func TestIngestRejectsExperienceWithoutProvenance(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `{
  "events": [
    {
      "experience_id": "exp_0144",
      "repo": "acme/billing-service",
      "recorded_at": "2026-04-04T18:21:00Z",
      "pr": 144,
      "commit": "abc9999",
      "paths": ["packages/billing/invoice.py"],
      "actor_kind": "human",
      "attribution_method": "manual",
      "outcome_kind": "merged_clean",
      "signature_class": "unknown",
      "check_name": "merge",
      "signature_key": "packages/billing/invoice.py",
      "extraction_confidence": "unknown"
    }
  ]
}`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "provenance_integrity_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestIngestGitHubLiveFailsClosedWithoutExplicitApprovals(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	t.Setenv("RELIA_GITHUB_TOKEN", "test-token")

	stdout, stderr, code := runForTest(t, []string{
		"--json",
		"ingest",
		"--github-live",
		"--repo", "acme/billing-service",
		"--pr", "302",
		"--github-token-env", "RELIA_GITHUB_TOKEN",
		"--github-token-scope", "read-only",
	}, false)

	if code != ExitCredential {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "credential_required" ||
		!strings.Contains(result.Errors[0].Message, "human approval") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-06.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("experience shard should not be persisted before live approval gates: %v", err)
	}
}

func TestIngestGitHubLiveDoesNotUseAmbientCredentials(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	t.Setenv("GITHUB_TOKEN", "ambient-token-must-not-be-used")

	stdout, stderr, code := runForTest(t, []string{
		"--json",
		"ingest",
		"--github-live",
		"--repo", "acme/billing-service",
		"--pr", "302",
		"--github-token-scope", "read-only",
		"--allow-network",
		"--allow-credentials",
		"--human-approved",
	}, false)

	if code != ExitCredential {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "credential_required" ||
		!strings.Contains(result.Errors[0].Message, "explicit --github-token-env") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestIngestGitHubLiveUsesBoundedRequestContext(t *testing.T) {
	ctx, cancel, client := githubLiveRequestContext()
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("github live request context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > githubLiveOverallTimeout {
		t.Fatalf("github live deadline remaining = %v, want within %v", remaining, githubLiveOverallTimeout)
	}
	if client == http.DefaultClient {
		t.Fatalf("github live client should not use http.DefaultClient")
	}
	if client.Timeout != githubLiveHTTPRequestTimeout {
		t.Fatalf("github live client timeout = %v, want %v", client.Timeout, githubLiveHTTPRequestTimeout)
	}
}

func TestIngestRejectsAgentSelfReportsBeforePersistence(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "self-report-outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "experience_id": "exp_self_report",
    "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
    "recorded_at": "2026-04-04T18:21:00Z",
    "pr": 144,
    "commit": "abc9999",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_tz_rollover",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/144"],
    "metadata": {"source_kind": "agent_self_report"}
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" ||
		!strings.Contains(result.Errors[0].Message, "self-reports") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("agent self-report should not be persisted: %v", err)
	}
}

func TestIngestRejectsNestedSourceMetadataBeforePersistence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		prefix   string
		metadata string
	}{
		{
			name:     "source_kind",
			metadata: `{"source": {"kind": "agent_reflection"}}`,
		},
		{
			name:     "source_type",
			metadata: `{"source": {"type": "agent_self_report"}}`,
		},
		{
			name:     "source_object_type",
			metadata: `{"source": {"object_type": "agent_reflection"}}`,
		},
		{
			name:     "metadata_event_type",
			metadata: `{"event_type": "agent_reflection"}`,
		},
		{
			name:     "camel_case_source_kind",
			metadata: `{"source_kind": "agentSelfReport"}`,
		},
		{
			name:     "camel_case_source_type",
			metadata: `{"source": {"type": "selfReported"}}`,
		},
		{
			name:   "object_type",
			prefix: `"object_type": "agent_self_report",`,
		},
		{
			name:   "event_type",
			prefix: `"event_type": "agent_reflection",`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := setupContractRepo(t)
			t.Chdir(tempDir)
			inputPath := filepath.Join(tempDir, "fixtures", "self-report-outcomes.json")
			writeFileForTest(t, inputPath, fmt.Sprintf(`[
	  {
	    "experience_id": "exp_%s",
	    %s
	    "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
	    "recorded_at": "2026-04-04T18:21:00Z",
	    "pr": 144,
    "commit": "abc9999",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
	    "signature_key": "tests/test_invoice.py::test_tz_rollover",
	    "extraction_confidence": "structured",
	    "provenance_urls": ["https://github.com/acme/billing-service/pull/144"],
	    "metadata": %s
	  }
	]`, tc.name, tc.prefix, metadataForSelfReportTest(tc.metadata)))

			stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

			if code != ExitValidation {
				t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if result.Errors[0].Type != "artifact_contract_validation_failed" ||
				!strings.Contains(result.Errors[0].Message, "self-reports") {
				t.Fatalf("errors = %#v", result.Errors)
			}
			if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("nested source metadata should not be persisted: %v", err)
			}
		})
	}
}

func metadataForSelfReportTest(metadata string) string {
	if metadata == "" {
		return `{}`
	}
	return metadata
}

func TestIngestInfersAttributionAndUpsertsIdempotently(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "repo": "acme/billing-service",
    "recorded_at": "2026-05-01T12:00:00Z",
    "pr": 210,
    "commit": "abc210",
    "paths": ["packages/billing/invoice.py"],
    "labels": ["agent-authored"],
    "outcome_kind": "merged_clean",
    "check_name": "merge",
    "signature_key": "packages/billing/invoice.py",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/210"]
  }
]`)

	for run := 0; run < 2; run++ {
		stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
		if code != ExitSuccess {
			t.Fatalf("run %d exit code = %d, stderr = %q, stdout = %q", run, code, stderr, stdout)
		}
		result := decodeResult(t, stdout)
		if got := int(result.Data["experiences_agent_attributed"].(float64)); got != 1 {
			t.Fatalf("run %d experiences_agent_attributed = %d", run, got)
		}
	}
	content, err := os.ReadFile(filepath.Join(tempDir, ".relia", "experiences", "2026-05.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	records := decodeJSONLines(t, string(content))
	if len(records) != 1 {
		t.Fatalf("idempotent upsert wrote %d records:\n%s", len(records), content)
	}
	attribution := records[0]["attribution"].(map[string]any)
	if attribution["actor_kind"] != "agent" || attribution["method"] != "pr_label" {
		t.Fatalf("attribution = %#v", attribution)
	}
	if records[0]["experience_id"] == "" {
		t.Fatalf("expected deterministic experience_id: %#v", records[0])
	}
}

func TestIngestGitHubOutcomesPersistsStructuredPRCheckRevertAndReviewRecords(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "github-outcomes.json")
	writeFileForTest(t, inputPath, `{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 301,
      "head_sha": "abc301",
      "html_url": "https://github.com/acme/billing-service/pull/301",
      "merged_at": "2026-06-01T12:00:00Z",
      "labels": ["agent-authored"],
      "files": [{"filename": "packages/billing/invoice.py"}],
      "check_runs": [{"name": "validate", "conclusion": "success"}]
    },
    {
      "number": 302,
      "head_sha": "abc302",
      "html_url": "https://github.com/acme/billing-service/pull/302",
      "merged_at": "2026-06-02T12:00:00Z",
      "labels": ["agent-authored"],
      "files": ["packages/billing/tax.py"],
      "check_runs": [
        {
          "name": "validate",
          "conclusion": "failure",
          "completed_at": "2026-06-02T12:10:00Z",
          "html_url": "https://github.com/acme/billing-service/actions/runs/302",
          "summary": "unit test failed",
          "paths": ["packages/billing/tax.py"]
        }
      ],
      "reverts": [
        {
          "created_at": "2026-06-03T12:00:00Z",
          "commit_sha": "def302",
          "commit_url": "https://github.com/acme/billing-service/commit/def302",
          "message": "Revert tax change",
          "paths": ["packages/billing/tax.py"]
        }
      ],
      "review_corrections": [
        {
          "marked": true,
          "resolved_at": "2026-06-04T12:00:00Z",
          "html_url": "https://github.com/acme/billing-service/pull/302",
          "path": "packages/billing/tax.py",
          "message": "Fix review finding"
        }
      ]
    }
  ]
}`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--github-outcomes", "--input", inputPath}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Data["source_format"] != "github_outcomes" {
		t.Fatalf("source_format = %#v", result.Data["source_format"])
	}
	if got := int(result.Data["experiences_persisted"].(float64)); got != 4 {
		t.Fatalf("experiences_persisted = %d", got)
	}
	content, err := os.ReadFile(filepath.Join(tempDir, ".relia", "experiences", "2026-06.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	records := decodeJSONLines(t, string(content))
	kinds := map[string]bool{}
	for _, record := range records {
		outcome := record["outcome"].(map[string]any)
		kind := outcome["kind"].(string)
		kinds[kind] = true
		metadata := record["metadata"].(map[string]any)
		if metadata["memory_source"] != "verified_outcome_event" {
			t.Fatalf("metadata = %#v", metadata)
		}
		if kind == "ci_failure" {
			signature := metadata["signature"].(map[string]any)
			if signature["key"] != "packages/billing/tax.py" {
				t.Fatalf("ci signature metadata = %#v", signature)
			}
		}
		if kind == "revert" {
			action := record["action"].(map[string]any)
			if action["commit"] != "abc302" {
				t.Fatalf("revert action = %#v", action)
			}
			if metadata["github_source_commit"] != "def302" {
				t.Fatalf("revert metadata = %#v", metadata)
			}
		}
	}
	for _, want := range []string{"merged_clean", "ci_failure", "revert", "review_correction"} {
		if !kinds[want] {
			t.Fatalf("missing outcome kind %q in records:\n%s", want, content)
		}
	}
}

func TestIngestGeneratedExperienceIDIncludesSignatureIdentity(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "repo": "acme/billing-service",
    "recorded_at": "2026-05-03T12:00:00Z",
    "pr": 212,
    "commit": "abc212",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/212"]
  },
  {
    "repo": "acme/billing-service",
    "recorded_at": "2026-05-03T12:05:00Z",
    "pr": 212,
    "commit": "abc212",
    "paths": ["packages/billing/tax.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_tax.py::test_rounding",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/212"]
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got := int(result.Data["experiences_persisted"].(float64)); got != 2 {
		t.Fatalf("experiences_persisted = %d", got)
	}
	content, err := os.ReadFile(filepath.Join(tempDir, ".relia", "experiences", "2026-05.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	records := decodeJSONLines(t, string(content))
	if len(records) != 2 {
		t.Fatalf("same PR/commit/outcome records should not overwrite each other, got %d:\n%s", len(records), content)
	}
	ids := map[string]bool{}
	for _, record := range records {
		id, ok := record["experience_id"].(string)
		if !ok || id == "" {
			t.Fatalf("record missing generated experience_id: %#v", record)
		}
		if ids[id] {
			t.Fatalf("duplicate generated experience_id %q in records:\n%s", id, content)
		}
		ids[id] = true
	}
}

func TestIngestSkipsUncertainAttributionByDefault(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "repo": "acme/billing-service",
    "recorded_at": "2026-05-02T12:00:00Z",
    "pr": 211,
    "commit": "abc211",
    "paths": ["packages/billing/invoice.py"],
    "outcome_kind": "merged_clean",
    "check_name": "merge",
    "signature_key": "packages/billing/invoice.py",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/211"]
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got := int(result.Data["experiences_persisted"].(float64)); got != 0 {
		t.Fatalf("experiences_persisted = %d", got)
	}
	if got := int(result.Data["experiences_skipped_uncertain"].(float64)); got != 1 {
		t.Fatalf("experiences_skipped_uncertain = %d", got)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-05.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uncertain experience should not be persisted: %v", err)
	}
}

func TestIngestInfersBotLoginFromMappedAgentAuthor(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "  agent_authors: []", `  agent_authors:
    - login: acme-claude-bot`)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "repo": "acme/billing-service",
    "recorded_at": "2026-05-04T12:00:00Z",
    "pr": 213,
    "commit": "abc213",
    "paths": ["packages/billing/invoice.py"],
    "actor": {"login": "acme-claude-bot"},
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/213"]
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got := int(result.Data["experiences_agent_attributed"].(float64)); got != 1 {
		t.Fatalf("experiences_agent_attributed = %d", got)
	}
	content, err := os.ReadFile(filepath.Join(tempDir, ".relia", "experiences", "2026-05.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	records := decodeJSONLines(t, string(content))
	attribution := records[0]["attribution"].(map[string]any)
	if attribution["actor_kind"] != "agent" || attribution["method"] != "bot_login" {
		t.Fatalf("attribution = %#v", attribution)
	}
}

func TestIngestDefaultsMissingOrInvalidUncertainPolicyToExclude(t *testing.T) {
	for _, tc := range []struct {
		name        string
		replacement string
	}{
		{name: "missing", replacement: ""},
		{name: "invalid", replacement: "  uncertain: exlcude\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := setupContractRepo(t)
			t.Chdir(tempDir)
			replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "  uncertain: exclude\n", tc.replacement)
			inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
			writeFileForTest(t, inputPath, `[
  {
    "repo": "acme/billing-service",
    "recorded_at": "2026-05-05T12:00:00Z",
    "pr": 214,
    "commit": "abc214",
    "paths": ["packages/billing/invoice.py"],
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/214"]
  }
]`)

			stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

			if code != ExitSuccess {
				t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if got := int(result.Data["experiences_persisted"].(float64)); got != 0 {
				t.Fatalf("experiences_persisted = %d", got)
			}
			if got := int(result.Data["experiences_skipped_uncertain"].(float64)); got != 1 {
				t.Fatalf("experiences_skipped_uncertain = %d", got)
			}
			if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-05.jsonl")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("uncertain experience should not be persisted: %v", err)
			}
		})
	}
}

func TestIngestReportsCorruptShardAsProvenanceIntegrity(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "experiences", "2026-05.jsonl"), "{not-json}\n")
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "repo": "acme/billing-service",
    "recorded_at": "2026-05-06T12:00:00Z",
    "pr": 215,
    "commit": "abc215",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/215"]
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "provenance_integrity_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestShardConsumersRejectSelfReportMarkersBeforeStructDecode(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			name: "backtest",
			args: []string{"--json", "backtest", "--window", "180d"},
		},
		{
			name: "distill",
			args: []string{"--json", "distill", "--format", "json"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := setupContractRepo(t)
			t.Chdir(tempDir)
			record := canonicalExperienceRecordMapForTest("exp_shard_self_report", 531)
			record["event_type"] = "agent_reflection"
			content, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			writeFileForTest(t, filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl"), string(content)+"\n")

			stdout, stderr, code := runForTest(t, tc.args, false)

			if code != ExitValidation {
				t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if result.Errors[0].Type != "artifact_contract_validation_failed" ||
				!strings.Contains(result.Errors[0].Message, "self-reports") {
				t.Fatalf("errors = %#v", result.Errors)
			}
		})
	}
}

func TestIngestRejectsFractionalPRNumbers(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "repo": "acme/billing-service",
    "recorded_at": "2026-05-06T12:00:00Z",
    "pr": 142.9,
    "commit": "abc142",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/142"]
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "provenance_integrity_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-05.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fractional PR experience should not be persisted: %v", err)
	}
}

func TestIngestPreservesLargeIntegerPRNumbers(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "repo": "acme/billing-service",
    "recorded_at": "2026-05-06T12:00:00Z",
    "pr": 9007199254740993,
    "commit": "abc142",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/9007199254740993"]
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	content, err := os.ReadFile(filepath.Join(tempDir, ".relia", "experiences", "2026-05.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte(`"pr":9007199254740993`)) {
		t.Fatalf("large integer PR was not preserved exactly:\n%s", content)
	}
}

func TestIngestPreservesCleanGitHubProvenanceURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		repo string
		url  string
	}{
		{
			name: "short owner repo",
			repo: "Clyra-AI/relia",
			url:  "https://github.com/Clyra-AI/relia/commit/de72faeb410006f9780c8cba1725674324d80156",
		},
		{
			name: "long owner repo",
			repo: "organizationname/billing-service",
			url:  "https://github.com/organizationname/billing-service/commit/de72faeb410006f9780c8cba1725674324d80156",
		},
		{
			name: "long mixed owner repo pull",
			repo: "Acme2026/SuperBillingServiceXYZ",
			url:  "https://github.com/Acme2026/SuperBillingServiceXYZ/pull/143",
		},
		{
			name: "long mixed owner repo actions run",
			repo: "Acme2026/SuperBillingServiceXYZ",
			url:  "https://github.com/Acme2026/SuperBillingServiceXYZ/actions/runs/143",
		},
		{
			name: "long mixed owner repo top level run",
			repo: "Acme2026/SuperBillingServiceXYZ",
			url:  "https://github.com/Acme2026/SuperBillingServiceXYZ/runs/143",
		},
		{
			name: "long mixed owner repo tree",
			repo: "Acme2026/SuperBillingServiceXYZ",
			url:  "https://github.com/Acme2026/SuperBillingServiceXYZ/tree/main",
		},
		{
			name: "long mixed owner repo blob",
			repo: "Acme2026/SuperBillingServiceXYZ",
			url:  "https://github.com/Acme2026/SuperBillingServiceXYZ/blob/main/README.md",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := setupContractRepo(t)
			t.Chdir(tempDir)
			inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
			writeFileForTest(t, inputPath, fmt.Sprintf(`[
  {
    "repo": %q,
    "recorded_at": "2026-05-06T12:00:00Z",
    "pr": 216,
    "commit": "de72faeb410006f9780c8cba1725674324d80156",
    "paths": ["cmd/relia/main.go"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "go-test",
    "signature_key": "cmd/relia/main_test.go::TestIngest",
    "extraction_confidence": "structured",
    "provenance_urls": [%q]
  }
]`, tc.repo, tc.url))

			stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

			if code != ExitSuccess {
				t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			content, err := os.ReadFile(filepath.Join(tempDir, ".relia", "experiences", "2026-05.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(content, []byte(tc.url)) {
				t.Fatalf("clean commit provenance URL was not preserved:\n%s", content)
			}
		})
	}
}

func TestIngestPreservesTopLevelCheckRunURLField(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	checkRunURL := "https://github.com/Acme2026/SuperBillingServiceXYZ/runs/143"
	writeFileForTest(t, inputPath, fmt.Sprintf(`[
  {
    "repo": "Acme2026/SuperBillingServiceXYZ",
    "recorded_at": "2026-05-06T12:00:00Z",
    "pr": 216,
    "commit": "de72faeb410006f9780c8cba1725674324d80156",
    "paths": ["cmd/relia/main.go"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "go-test",
    "signature_key": "cmd/relia/main_test.go::TestIngest",
    "extraction_confidence": "structured",
    "check_run_url": %q
  }
]`, checkRunURL))

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	content, err := os.ReadFile(filepath.Join(tempDir, ".relia", "experiences", "2026-05.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte(checkRunURL)) {
		t.Fatalf("clean check_run_url was not preserved:\n%s", content)
	}
}

func TestIngestDoesNotPartiallyWriteShardsWhenLaterShardIsCorrupt(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	januaryShard := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	originalJanuary := `{"experience_id":"existing_january"}
`
	writeFileForTest(t, januaryShard, originalJanuary)
	februaryShard := filepath.Join(tempDir, ".relia", "experiences", "2026-02.jsonl")
	writeFileForTest(t, februaryShard, "{not-json}\n")
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "experience_id": "exp_0101",
    "repo": "acme/billing-service",
    "recorded_at": "2026-01-06T12:00:00Z",
    "pr": 216,
    "commit": "abc216",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/216"]
  },
  {
    "experience_id": "exp_0201",
    "repo": "acme/billing-service",
    "recorded_at": "2026-02-06T12:00:00Z",
    "pr": 217,
    "commit": "abc217",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "terminal_state": "failed",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/217"]
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "provenance_integrity_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	januaryContent, err := os.ReadFile(januaryShard)
	if err != nil {
		t.Fatal(err)
	}
	if string(januaryContent) != originalJanuary {
		t.Fatalf("january shard was partially updated:\n%s", januaryContent)
	}
	februaryContent, err := os.ReadFile(februaryShard)
	if err != nil {
		t.Fatal(err)
	}
	if string(februaryContent) != "{not-json}\n" {
		t.Fatalf("february shard changed unexpectedly:\n%s", februaryContent)
	}
}
