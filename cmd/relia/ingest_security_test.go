package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIngestPersistsCanonicalExperienceWithRedactionAndProvenance(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0142","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-02T18:21:00Z","pr":142,"commit":"abc1234","paths":["packages/billing/invoice.py","tests/test_invoice.py"],"actor_kind":"agent","attribution_method":"coauthor_trailer","attribution_confidence":0.91,"outcome_kind":"ci_failure","terminal_state":"failed","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/test_invoice.py::test_tz_rollover","extraction_confidence":"structured","message":"Authorization failed for Bearer ghp_1234567890abcdef1234567890abcdef123456","provenance_urls":["https://github.com/acme/billing-service/pull/142","https://github.com/acme/billing-service/actions/runs/981"],"metadata":{"raw_log":"token ghp_1234567890abcdef1234567890abcdef123456 was rejected"}}
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "ingest" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	if result.RedactionStatus != "applied" {
		t.Fatalf("redaction_status = %q", result.RedactionStatus)
	}
	if got := int(result.Data["experiences_persisted"].(float64)); got != 1 {
		t.Fatalf("experiences_persisted = %d", got)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl")
	content, err := os.ReadFile(shardPath)
	if err != nil {
		t.Fatalf("expected persisted experience shard: %v", err)
	}
	if bytes.Contains(content, []byte("ghp_1234567890abcdef")) {
		t.Fatalf("persisted shard contains unredacted token:\n%s", content)
	}
	if !bytes.Contains(content, []byte("[REDACTED:token]")) {
		t.Fatalf("persisted shard missing token redaction:\n%s", content)
	}
	records := decodeJSONLines(t, string(content))
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	record := records[0]
	if record["object_type"] != "relia.experience_record" {
		t.Fatalf("object_type = %#v", record["object_type"])
	}
	if record["schema_version"] != "1.0" {
		t.Fatalf("schema_version = %#v", record["schema_version"])
	}
	if record["redaction_status"] != "applied" {
		t.Fatalf("record redaction_status = %#v", record["redaction_status"])
	}
	repo := record["repo"].(map[string]any)
	if repo["owner"] != "acme" || repo["name"] != "billing-service" {
		t.Fatalf("repo = %#v", repo)
	}
	attribution := record["attribution"].(map[string]any)
	if attribution["actor_kind"] != "agent" || attribution["method"] != "coauthor_trailer" {
		t.Fatalf("attribution = %#v", attribution)
	}
	provenance := record["provenance"].(map[string]any)
	if urls := provenance["urls"].([]any); len(urls) != 2 {
		t.Fatalf("provenance urls = %#v", urls)
	}
	metadata := record["metadata"].(map[string]any)
	if _, err := time.Parse(time.RFC3339, metadata["last_ingest_at"].(string)); err != nil {
		t.Fatalf("metadata.last_ingest_at = %#v, err = %v", metadata["last_ingest_at"], err)
	}
	if metadata["merged_prs_since_last_ingest"] != float64(0) {
		t.Fatalf("metadata.merged_prs_since_last_ingest = %#v", metadata["merged_prs_since_last_ingest"])
	}
}

func TestIngestRedactsPluralSecretFieldsBeforePersistence(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "experience_id": "exp_0142_secret",
    "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
    "recorded_at": "2026-04-02T18:21:00Z",
    "pr": 142,
    "commit": "abc1234",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_tz_rollover",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/142"],
    "metadata": {"secrets": ["short-password", "low-token"]}
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	content, err := os.ReadFile(filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl"))
	if err != nil {
		t.Fatalf("expected persisted experience shard: %v", err)
	}
	if bytes.Contains(content, []byte("short-password")) || bytes.Contains(content, []byte("low-token")) {
		t.Fatalf("plural secret field was persisted without redaction:\n%s", content)
	}
	records := decodeJSONLines(t, string(content))
	metadata := records[0]["metadata"].(map[string]any)
	if metadata["secrets"] != "[REDACTED:secret]" {
		t.Fatalf("metadata.secrets = %#v", metadata["secrets"])
	}
}

func TestIngestFailsClosedBeforePersistenceForUnrecognizedSecret(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "experience_id": "exp_0143",
    "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
    "recorded_at": "2026-04-03T18:21:00Z",
    "pr": 143,
    "commit": "def5678",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/143"],
    "metadata": {"opaque": "z6MvN2p9QxR4sT8aK3vY7bL0cD5eF1gH2jP9mQ4rS6tU"}
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

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
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("experience shard should not be persisted on redaction failure: %v", err)
	}
}

func TestIngestGitHubOutcomesFailsClosedBeforeDroppingRawSecret(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "github-outcomes.json")
	writeFileForTest(t, inputPath, `{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 144,
      "head_sha": "def144",
      "html_url": "https://github.com/acme/billing-service/pull/144",
      "files": [{"filename": "packages/billing/invoice.py"}],
      "merged_at": "2026-06-04T12:00:00Z",
      "body": "unused raw field z6MvN2p9QxR4sT8aK3vY7bL0cD5eF1gH2jP9mQ4rS6tU"
    }
  ]
}`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--github-outcomes", "--input", inputPath}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "redaction_safety_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-06.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("experience shard should not be persisted on raw GitHub export redaction failure: %v", err)
	}
}

func TestIngestGitHubOutcomesAllowsRawGitHubNodeID(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "github-outcomes.json")
	rawNodeID := "MDEyOlB1bGxSZXF1ZXN0MTIzNDU2Nzg5MDEyMzQ1Njc4OTA="
	writeFileForTest(t, inputPath, `{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 145,
      "node_id": "`+rawNodeID+`",
      "head_sha": "def145",
      "html_url": "https://github.com/acme/billing-service/pull/145",
      "labels": [{"name": "agent-authored"}],
      "files": [{"filename": "packages/billing/invoice.py"}],
      "merged_at": "2026-06-04T12:00:00Z"
    }
  ]
}`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--github-outcomes", "--input", inputPath}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	content, err := os.ReadFile(filepath.Join(tempDir, ".relia", "experiences", "2026-06.jsonl"))
	if err != nil {
		t.Fatalf("expected persisted experience shard: %v", err)
	}
	if bytes.Contains(content, []byte(rawNodeID)) {
		t.Fatalf("raw GitHub node_id should not be persisted:\n%s", content)
	}
}

func TestIngestFailsClosedBeforePersistenceForSecretShapedMetadataKey(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "experience_id": "exp_0143_key_secret",
    "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
    "recorded_at": "2026-04-03T18:21:00Z",
    "pr": 143,
    "commit": "def5678",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/143"],
    "metadata": {"ghp_1234567890abcdef1234567890abcdef123456": "seen"}
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "redaction_safety_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("experience shard should not be persisted on secret-shaped metadata key: %v", err)
	}
}

func TestIngestFailsClosedForUnsafeProvenanceURLQuery(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "experience_id": "exp_0143_url_secret",
    "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
    "recorded_at": "2026-04-03T18:21:00Z",
    "pr": 143,
    "commit": "def5678",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/143?token=z6MvN2p9QxR4sT8aK3vY7bL0cD5eF1gH2jP9mQ4rS6tU"]
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "redaction_safety_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("experience shard should not be persisted on unsafe provenance URL: %v", err)
	}
}

func TestIngestRejectsNonCanonicalGitHubProvenanceURL(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "experience_id": "exp_0143_url_fragment",
    "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
    "recorded_at": "2026-04-03T18:21:00Z",
    "pr": 143,
    "commit": "def5678",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/143?from=browser#discussion_r1"]
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "provenance_integrity_failed" || !strings.Contains(result.Errors[0].Message, "canonical") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("experience shard should not be persisted on non-canonical provenance URL: %v", err)
	}
}

func TestIngestFailsClosedForUnsafeProvenanceURLPathSegment(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
	}{
		{
			name: "actions run",
			url:  "https://github.com/acme/billing-service/actions/runs/z6MvN2p9QxR4sT8aK3vY7bL0cD5eF1gH2jP9mQ4rS6tU",
		},
		{
			name: "appended after pr",
			url:  "https://github.com/acme/billing-service/pull/143/z6MvN2p9QxR4sT8aK3vY7bL0cD5eF1gH2jP9mQ4rS6tU",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := setupContractRepo(t)
			t.Chdir(tempDir)
			inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
			writeFileForTest(t, inputPath, fmt.Sprintf(`[
  {
    "experience_id": "exp_0143_url_path_secret",
    "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
    "recorded_at": "2026-04-03T18:21:00Z",
    "pr": 143,
    "commit": "def5678",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": [%q]
  }
]`, tc.url))

			stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

			if code != ExitRedactionSafety {
				t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if result.Errors[0].Type != "redaction_safety_failed" {
				t.Fatalf("error type = %q", result.Errors[0].Type)
			}
			if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("experience shard should not be persisted on unsafe provenance URL path: %v", err)
			}
		})
	}
}

func TestIngestFailsClosedForSlashBearingProvenanceURLPathSecret(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
	}{
		{
			name: "raw slash",
			url:  "https://github.com/acme/billing-service/actions/runs/z6MvN2p9QxR4sT8a/K3vY7bL0cD5eF1gH2jP9mQ4rS6tU",
		},
		{
			name: "encoded slash",
			url:  "https://github.com/acme/billing-service/actions/runs/z6MvN2p9QxR4sT8a%2FK3vY7bL0cD5eF1gH2jP9mQ4rS6tU",
		},
		{
			name: "short path",
			url:  "https://github.com/z6MvN2p9QxR4sT8a/K3vY7bL0cD5eF1gH2jP9mQ4rS6tU",
		},
		{
			name: "short middle fragment",
			url:  "https://github.com/acme/billing-service/actions/runs/z6MvN2p9QxR4sT8a/abc/K3vY7bL0cD5eF1gH",
		},
		{
			name: "weak leading fragment",
			url:  "https://github.com/acme/billing-service/actions/runs/qazwsxedcrfvtgby/K3vY7bL0cD5eF1gH2jP9mQ",
		},
		{
			name: "encoded weak leading fragment",
			url:  "https://github.com/acme/billing-service/actions/runs/qazwsxedcrfvtgby%2FK3vY7bL0cD5eF1gH2jP9mQ",
		},
		{
			name: "route like middle fragment",
			url:  "https://github.com/acme/billing-service/actions/runs/z6MvN2p9QxR4sT8a/pull/K3vY7bL0cD5eF1gH",
		},
		{
			name: "route word threshold fragment",
			url:  "https://github.com/acme/billing-service/actions/runs/z6MvN2p9QxR4sT/pull/8aK3vY7bL0cD5e",
		},
		{
			name: "weak fragment split by route word",
			url:  "https://github.com/qazwsxedcrfvtgby/commit/K3vY7bL0cD5eF1gH",
		},
		{
			name: "invalid commit route payload split token",
			url:  "https://github.com/acme/billing-service/commit/qazwsxedcrfvtgby/K3vY7bL0cD5eF1gH",
		},
		{
			name: "fully chunked short fragments",
			url:  "https://github.com/acme/billing-service/actions/runs/z6MvN2p9/QxR4sT8a/K3vY7bL0/cD5eF1gH",
		},
		{
			name: "owner repo fragments before route boundary",
			url:  "https://github.com/z6MvN2p9QxR4sT8a/abc/actions/runs/K3vY7bL0cD5eF1gH",
		},
		{
			name: "decimal middle fragment",
			url:  "https://github.com/acme/billing-service/actions/runs/z6MvN2p9QxR4sT8a/143/K3vY7bL0cD5eF1gH",
		},
		{
			name: "decimal trailing fragment",
			url:  "https://github.com/acme/billing-service/actions/runs/QwErTyUiOpAsDfGhJkLzXcVb/1234567890",
		},
		{
			name: "decimal leading fragment",
			url:  "https://github.com/acme/billing-service/actions/runs/1234567890/QwErTyUiOpAsDfGhJkLzXcVb",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := setupContractRepo(t)
			t.Chdir(tempDir)
			inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
			writeFileForTest(t, inputPath, fmt.Sprintf(`[
  {
    "experience_id": "exp_0143_url_path_slash_secret",
    "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
    "recorded_at": "2026-04-03T18:21:00Z",
    "pr": 143,
    "commit": "def5678",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": [%q]
  }
]`, tc.url))

			stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

			if code != ExitRedactionSafety {
				t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if result.Errors[0].Type != "redaction_safety_failed" {
				t.Fatalf("error type = %q", result.Errors[0].Type)
			}
			if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("experience shard should not be persisted on slash-bearing provenance URL path secret: %v", err)
			}
		})
	}
}

func TestIngestScansMalformedCommitBeforePersistence(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.json")
	writeFileForTest(t, inputPath, `[
  {
    "experience_id": "exp_0143_commit",
    "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
    "recorded_at": "2026-04-03T18:21:00Z",
    "pr": 143,
    "commit": "z6MvN2p9QxR4sT8aK3vY7bL0cD5eF1gH2jP9mQ4rS6tU",
    "paths": ["packages/billing/invoice.py"],
    "actor_kind": "agent",
    "attribution_method": "manual",
    "outcome_kind": "ci_failure",
    "signature_class": "test_failure",
    "check_name": "pytest-billing",
    "signature_key": "tests/test_invoice.py::test_total",
    "extraction_confidence": "structured",
    "provenance_urls": ["https://github.com/acme/billing-service/pull/143"]
  }
]`)

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "redaction_safety_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "experiences", "2026-04.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("experience shard should not be persisted on malformed high-entropy commit: %v", err)
	}
}
