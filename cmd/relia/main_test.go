package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
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
	if result.Metadata["schema_ref"] != "schemas/command-result.schema.json" {
		t.Fatalf("metadata = %#v", result.Metadata)
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
		{"--json", "compile"},
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

func TestBacktestComputesConservativeERRWithFlakesPossibleAndStaleBaseline(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py","tests/billing/test_invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py","tests/billing/test_invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
		`{"experience_id":"exp_0003","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-20T10:00:00Z","pr":103,"commit":"abc003","paths":["packages/billing/invoice.py","tests/billing/test_invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"log_parsed_low","provenance_urls":["https://github.com/acme/billing-service/pull/103"]}`,
		`{"experience_id":"exp_0101","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-02-01T10:00:00Z","pr":201,"commit":"abc101","paths":["packages/notifications/logging.py","tests/notifications/test_retry.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_external_retry","signature_class":"test_failure","check_name":"pytest-notifications","signature_key":"tests/notifications/test_retry.py::test_retry","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/201"]}`,
		`{"experience_id":"exp_0102","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-02-02T10:00:00Z","pr":202,"commit":"abc102","paths":["packages/worker/retry_queue.py","tests/notifications/test_retry.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_external_retry","signature_class":"test_failure","check_name":"pytest-notifications","signature_key":"tests/notifications/test_retry.py::test_retry","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/202"]}`,
		`{"experience_id":"exp_0103","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-02-03T10:00:00Z","pr":203,"commit":"abc103","paths":["packages/notifications/client.py","tests/notifications/test_retry.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_external_retry","signature_class":"test_failure","check_name":"pytest-notifications","signature_key":"tests/notifications/test_retry.py::test_retry","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/203"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json"), `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.1,
  "metadata": {
    "source_artifact_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
  }
}
`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	report := decodeBacktestReportFromResult(t, result)
	if result.RedactionStatus != "applied" {
		t.Fatalf("redaction_status = %q, want applied", result.RedactionStatus)
	}
	if report.ObjectType != "relia.recurrence_report" || report.SchemaVersion != commandSchemaVersion {
		t.Fatalf("report contract = %#v", report)
	}
	if report.Metrics.PRsAnalyzed != 6 || report.Metrics.AgentAttributedPRs != 6 {
		t.Fatalf("metrics = %#v, want six analyzed and agent-attributed PRs", report.Metrics)
	}
	if report.Metrics.AgentFailuresByOutcomeKind["ci_failure"] != 6 {
		t.Fatalf("agent failure breakdown = %#v", report.Metrics.AgentFailuresByOutcomeKind)
	}
	if report.Summary.AgentFailureDenominator != 6 {
		t.Fatalf("denominator = %d, want 6 total agent-attributed failures including flake-discounted rows", report.Summary.AgentFailureDenominator)
	}
	if report.Summary.ConfirmedRecurrenceCount != 1 || len(report.ConfirmedRecurrences) != 1 {
		t.Fatalf("confirmed recurrences = %#v", report.ConfirmedRecurrences)
	}
	if report.Summary.PossibleRecurrenceCount != 1 || len(report.PossibleRecurrences) != 1 {
		t.Fatalf("possible recurrences = %#v", report.PossibleRecurrences)
	}
	if report.HeadlineERR != 0.1667 {
		t.Fatalf("headline_err = %.4f, want confirmed-only numerator over 6 agent-attributed failures", report.HeadlineERR)
	}
	if report.ConfirmedRecurrences[0].PriorExperienceID != "exp_0001" || report.ConfirmedRecurrences[0].CurrentExperienceID != "exp_0002" {
		t.Fatalf("confirmed pair = %#v", report.ConfirmedRecurrences[0])
	}
	if report.ConfirmedRecurrences[0].PriorURL == "" || report.ConfirmedRecurrences[0].CurrentURL == "" || len(report.ConfirmedRecurrences[0].Refs) != 2 {
		t.Fatalf("confirmed pair missing resolvable links and refs: %#v", report.ConfirmedRecurrences[0])
	}
	if report.PossibleRecurrences[0].Confidence != "possible" || !strings.Contains(report.PossibleRecurrences[0].Reason, "excluded") {
		t.Fatalf("possible pair = %#v", report.PossibleRecurrences[0])
	}
	if report.Summary.FlakeDiscountedCount != 3 || len(report.FlakeDiscounts) != 3 {
		t.Fatalf("flake discounts = %#v", report.FlakeDiscounts)
	}
	if report.Baseline.Status != "stale" || !report.Baseline.Stale {
		t.Fatalf("baseline = %#v, want stale", report.Baseline)
	}
	if report.Gate.Enabled || report.Gate.Status != "off" {
		t.Fatalf("gate = %#v, want off by default", report.Gate)
	}
	if len(report.TopRepeatedMistakes) != 1 ||
		report.TopRepeatedMistakes[0].SignatureID != "class_key:test_failure:tests/billing/test_invoice.py::test_clock" ||
		report.TopRepeatedMistakes[0].RepeatCount != 1 ||
		!stringSlicesEqual(report.TopRepeatedMistakes[0].ExperienceIDs, []string{"exp_0001", "exp_0002"}) {
		t.Fatalf("top repeated mistakes = %#v", report.TopRepeatedMistakes)
	}
	if report.OperatorFeedback.Summary == "" ||
		!strings.Contains(report.OperatorFeedback.ConservativeMatchingNote, "confirmed") ||
		report.OperatorFeedback.NextCommand != "relia distill --format json" {
		t.Fatalf("operator feedback = %#v", report.OperatorFeedback)
	}
	if report.Badge.Label != "Relia" ||
		report.Badge.Message != "ERR 16.7%" ||
		report.Badge.Status != "current" ||
		report.Badge.Stale ||
		report.Badge.Color != "yellow" ||
		!strings.Contains(report.Badge.Reason, "ingest metadata") {
		t.Fatalf("badge = %#v", report.Badge)
	}
	if report.Metadata["last_ingest_at"] == "" || report.Metadata["merged_prs_since_last_ingest"] != float64(0) {
		t.Fatalf("badge freshness metadata = %#v", report.Metadata)
	}
	assertReportDiagnosticTypes(t, report.Diagnostics, []string{
		"memory_source_verified",
		"possible_recurrences_excluded",
		"flake_discounts_visible",
		"stale_baseline",
	})
	jsonPath, _ := result.Data["json_report_path"].(string)
	htmlPath, _ := result.Data["html_report_path"].(string)
	if jsonPath == "" || htmlPath == "" {
		t.Fatalf("report artifact paths missing from result data: %#v", result.Data)
	}
	if result.Data["report_path"] != htmlPath ||
		result.Data["error_recurrence_rate"] != report.HeadlineERR ||
		result.Data["baseline_ref"] != ".relia/baselines/error-recurrence-baseline.json" {
		t.Fatalf("result data did not expose report metrics and refs: %#v", result.Data)
	}
	jsonContent, err := os.ReadFile(filepath.Join(tempDir, filepath.FromSlash(jsonPath)))
	if err != nil {
		t.Fatalf("read json report: %v", err)
	}
	if !bytes.Contains(jsonContent, []byte(`"object_type": "relia.recurrence_report"`)) {
		t.Fatalf("json report missing recurrence object:\n%s", jsonContent)
	}
	htmlContent, err := os.ReadFile(filepath.Join(tempDir, filepath.FromSlash(htmlPath)))
	if err != nil {
		t.Fatalf("read html report: %v", err)
	}
	if !bytes.Contains(htmlContent, []byte("Possible Recurrences")) {
		t.Fatalf("html report missing possible recurrence section:\n%s", htmlContent)
	}
	if !bytes.Contains(htmlContent, []byte("Top Repeated Mistakes")) ||
		!bytes.Contains(htmlContent, []byte("Badge: Relia ERR 16.7%")) {
		t.Fatalf("html report missing operator summary and badge:\n%s", htmlContent)
	}
}

func TestBuildReportBadgeComputesFreshness(t *testing.T) {
	report := recurrenceReport{
		ReportID:    "backtest_fresh",
		Window:      recurrenceWindow{End: "2026-01-20T00:00:00Z"},
		Summary:     recurrenceSummary{HeadlineERRPercent: "4.1%"},
		HeadlineERR: 0.041,
		Metadata: map[string]any{
			"last_ingest_at":               "2026-06-20T00:00:00Z",
			"merged_prs_since_last_ingest": 0,
		},
	}
	now := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	badge := buildReportBadgeAt(report, now)
	if badge.Status != "current" || badge.Stale || badge.Message != "ERR 4.1%" || badge.Color != "brightgreen" {
		t.Fatalf("fresh badge = %#v, want current", badge)
	}
	if !strings.Contains(badge.Reason, "ingest metadata") {
		t.Fatalf("fresh badge reason = %q", badge.Reason)
	}

	report.Metadata["last_ingest_at"] = "2026-05-29T00:00:00Z"
	badge = buildReportBadgeAt(report, now)
	if badge.Status != "stale" || !badge.Stale || badge.Message != "ERR 4.1% stale" || badge.Color != "lightgrey" {
		t.Fatalf("old badge = %#v, want stale", badge)
	}
	if !strings.Contains(badge.Reason, "Last ingest exceeds") {
		t.Fatalf("old badge reason = %q", badge.Reason)
	}

	report.Metadata = map[string]any{
		"merged_prs_since_last_ingest": 0,
	}
	badge = buildReportBadgeAt(report, now)
	if badge.Status != "stale" || !badge.Stale {
		t.Fatalf("missing ingest badge = %#v, want stale", badge)
	}
	if !strings.Contains(badge.Reason, "Ingest freshness is unavailable") {
		t.Fatalf("missing ingest badge reason = %q", badge.Reason)
	}
}

func TestBuildReportBadgeComputesActivityStaleness(t *testing.T) {
	report := recurrenceReport{
		ReportID: "backtest_activity",
		Window:   recurrenceWindow{End: "2026-06-20T00:00:00Z"},
		Summary:  recurrenceSummary{HeadlineERRPercent: "4.1%"},
		Metadata: map[string]any{
			"last_ingest_at":               "2026-06-20T00:00:00Z",
			"merged_prs_since_last_ingest": float64(21),
		},
	}
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)

	badge := buildReportBadgeAt(report, now)
	if badge.Status != "stale" || !badge.Stale || badge.Message != "ERR 4.1% stale" {
		t.Fatalf("activity badge = %#v, want stale", badge)
	}
	if !strings.Contains(badge.Reason, "20 PRs") {
		t.Fatalf("activity badge reason = %q", badge.Reason)
	}

	report.Metadata = map[string]any{
		"last_ingest_at": "2026-06-20T00:00:00Z",
	}
	badge = buildReportBadgeAt(report, now)
	if badge.Status != "stale" || !badge.Stale {
		t.Fatalf("missing activity badge = %#v, want stale", badge)
	}
	if !strings.Contains(badge.Reason, "activity freshness is unavailable") {
		t.Fatalf("missing activity badge reason = %q", badge.Reason)
	}

	report.Metadata = map[string]any{
		"last_ingest_at":               "2026-06-20T00:00:00Z",
		"merged_prs_since_last_ingest": json.Number("-1"),
	}
	badge = buildReportBadgeAt(report, now)
	if badge.Status != "stale" || !badge.Stale {
		t.Fatalf("negative activity badge = %#v, want stale", badge)
	}
	if !strings.Contains(badge.Reason, "activity freshness is unavailable") {
		t.Fatalf("negative activity badge reason = %q", badge.Reason)
	}
}

func TestBacktestCommandResultCountsAgentAttributedExperiencesSeparatelyFromPRs(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_invoice_tax","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_tax","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-02T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/payment.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"review_correction","terminal_state":"corrected","signature_id":"sig_payment_rounding","signature_class":"review_correction","check_name":"review","signature_key":"packages/billing/payment.py::rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	report := decodeBacktestReportFromResult(t, result)
	if got := int(result.Data["experiences_agent_attributed"].(float64)); got != 2 {
		t.Fatalf("experiences_agent_attributed = %d, want two agent-attributed records", got)
	}
	if got := int(result.Data["agent_attributed_prs"].(float64)); got != 1 {
		t.Fatalf("agent_attributed_prs = %d, want one unique agent-attributed PR", got)
	}
	if report.Metrics.AgentAttributedExperiences != 2 || report.Metrics.AgentAttributedPRs != 1 {
		t.Fatalf("metrics = %#v, want separate experience and PR counts", report.Metrics)
	}
}

func TestBacktestInteractiveOutputShowsOperatorSummaryAndBadge(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"backtest", "--window", "180d"}, true)

	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	if json.Valid([]byte(stdout)) {
		t.Fatalf("interactive backtest output should be human-readable, got JSON: %s", stdout)
	}
	for _, want := range []string{
		"PRs analyzed: 2",
		"Confirmed recurrences: 1",
		"Top repeated mistakes:",
		"Error recurrence rate: 50.0%",
		"Badge: Relia ERR 50.0%",
		"Report: .relia/reports/",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("interactive backtest output missing %q:\n%s", want, stdout)
		}
	}
}

func TestBacktestAutoFlakeUsesCanonicalSignatureKeys(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_pytest","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/worker/clock.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_go_test","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
		`{"experience_id":"exp_0003","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-20T10:00:00Z","pr":103,"commit":"abc003","paths":["packages/notifications/clock.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_actions","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/103"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.Summary.AgentFailureDenominator != 3 || report.Summary.FlakeDiscountedCount != 3 || len(report.FlakeDiscounts) != 3 {
		t.Fatalf("summary = %#v flakes = %#v, want three canonical-key flakes retained in denominator", report.Summary, report.FlakeDiscounts)
	}
	for _, flake := range report.FlakeDiscounts {
		if len(flake.SupportingPRs) != 2 || len(flake.SupportingRefs) != 2 {
			t.Fatalf("flake = %#v, want canonical-key supporting PRs and refs despite different generated ids", flake)
		}
	}
	if report.Summary.ConfirmedRecurrenceCount != 0 || report.Summary.PossibleRecurrenceCount != 0 || report.HeadlineERR != 0 {
		t.Fatalf("recurrences confirmed=%d possible=%d headline=%.4f, want flakes excluded from recurrence scoring", report.Summary.ConfirmedRecurrenceCount, report.Summary.PossibleRecurrenceCount, report.HeadlineERR)
	}
}

func TestBacktestBaselineAcceptsSummaryHeadlineERR(t *testing.T) {
	tempDir := t.TempDir()
	baselinePath := filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json")
	currentWindow := recurrenceWindow{Start: "2026-01-01T00:00:00Z", End: "2026-01-31T00:00:00Z"}
	writeFileForTest(t, baselinePath, `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "summary": {
    "headline_err": 0.25
  },
  "window": {
    "start": "2026-01-01T00:00:00Z",
    "end": "2026-01-31T00:00:00Z"
  },
  "metadata": {
    "source_artifact_digest": "sha256:baseline"
  }
}
`)

	baseline, commandErr := compareBacktestBaseline(tempDir, ".relia/baselines/error-recurrence-baseline.json", 0.5, "sha256:baseline", currentWindow)

	if commandErr != nil {
		t.Fatalf("compare baseline returned error: %#v", commandErr)
	}
	if baseline.Status != "current" || baseline.Stale {
		t.Fatalf("baseline status = %#v, want current", baseline)
	}
	if baseline.HeadlineERR != 0.25 || baseline.Delta != 0.25 {
		t.Fatalf("baseline values = %#v, want headline 0.25 and delta 0.25", baseline)
	}
}

func TestBacktestBaselineMarksWindowMismatchStale(t *testing.T) {
	tempDir := t.TempDir()
	baselinePath := filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json")
	writeFileForTest(t, baselinePath, `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.25,
  "window": {
    "start": "2026-01-01T00:00:00Z",
    "end": "2026-06-29T00:00:00Z"
  },
  "metadata": {
    "source_artifact_digest": "sha256:baseline"
  }
}
`)

	currentWindow := recurrenceWindow{Start: "2026-06-01T00:00:00Z", End: "2026-06-29T00:00:00Z"}
	baseline, commandErr := compareBacktestBaseline(tempDir, ".relia/baselines/error-recurrence-baseline.json", 0.5, "sha256:baseline", currentWindow)

	if commandErr != nil {
		t.Fatalf("compare baseline returned error: %#v", commandErr)
	}
	if baseline.Status != "stale" || !baseline.Stale || !strings.Contains(baseline.Reason, "window") {
		t.Fatalf("baseline status = %#v, want stale window mismatch", baseline)
	}
	if baseline.HeadlineERR != 0.25 || baseline.Delta != 0.25 {
		t.Fatalf("baseline values = %#v, want headline 0.25 and delta 0.25", baseline)
	}
}

func TestBacktestBaselineJSONPreservesZeroMetrics(t *testing.T) {
	current := baselineComparison{
		Status:      "current",
		Path:        ".relia/baselines/error-recurrence-baseline.json",
		HeadlineERR: 0,
		Delta:       0,
		Stale:       false,
		Reason:      "Saved baseline was computed from the same source artifact digest.",
	}
	currentJSON, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(currentJSON, []byte(`"headline_err":0`)) || !bytes.Contains(currentJSON, []byte(`"delta":0`)) {
		t.Fatalf("current baseline JSON = %s, want explicit zero metrics", currentJSON)
	}

	missing := baselineComparison{
		Status: "missing",
		Path:   ".relia/baselines/error-recurrence-baseline.json",
		Stale:  false,
		Reason: "No saved ERR baseline exists yet; use --save-baseline after reviewing the report to create one.",
	}
	missingJSON, err := json.Marshal(missing)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(missingJSON, []byte("headline_err")) || bytes.Contains(missingJSON, []byte("delta")) {
		t.Fatalf("missing baseline JSON = %s, want omitted comparison metrics", missingJSON)
	}
}

func TestBacktestReportSerializesEmptyCollectionsAsArrays(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	reportData, ok := result.Data["report"].(map[string]any)
	if !ok {
		t.Fatalf("report data = %#v, want object", result.Data["report"])
	}
	for _, field := range []string{"confirmed_recurrences", "possible_recurrences", "flake_discounts", "attribution_uncertain"} {
		values, ok := reportData[field].([]any)
		if !ok || len(values) != 0 {
			t.Fatalf("report[%s] = %#v, want empty JSON array", field, reportData[field])
		}
	}
	report := decodeBacktestReportFromResult(t, result)
	if report.Summary.AgentFailureDenominator != 1 || report.HeadlineERR != 0 {
		t.Fatalf("report summary = %#v headline=%.4f, want one denominator and zero ERR", report.Summary, report.HeadlineERR)
	}
}

func TestBacktestSaveBaselineReportsFreshCurrentValues(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json"), `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.1,
  "window": {
    "start": "2025-01-01T00:00:00Z",
    "end": "2025-01-31T00:00:00Z"
  },
  "metadata": {
    "source_artifact_digest": "sha256:stale"
  }
}
`)
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d", "--save-baseline"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.HeadlineERR != 0.5 {
		t.Fatalf("headline_err = %.4f, want 0.5", report.HeadlineERR)
	}
	if report.Baseline.Status != "saved" || report.Baseline.Stale || report.Baseline.HeadlineERR != report.HeadlineERR || report.Baseline.Delta != 0 {
		t.Fatalf("baseline = %#v, want freshly saved current values", report.Baseline)
	}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Type == "stale_baseline" {
			t.Fatalf("diagnostics retained stale baseline after save: %#v", report.Diagnostics)
		}
	}
	baselineContent, err := os.ReadFile(filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(baselineContent, []byte(`"headline_err": 0.5`)) {
		t.Fatalf("saved baseline missing current headline ERR:\n%s", baselineContent)
	}
}

func TestBacktestBaselineDigestIgnoresIngestFreshnessMetadata(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d", "--save-baseline"}, false)
	if code != ExitSuccess {
		t.Fatalf("save baseline exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	savedReport := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	savedDigest := stringFromAny(savedReport.Metadata["source_artifact_digest"])
	if savedDigest == "" {
		t.Fatalf("saved report missing source digest: %#v", savedReport.Metadata)
	}

	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	shardContent, err := os.ReadFile(shardPath)
	if err != nil {
		t.Fatal(err)
	}
	records := decodeJSONLines(t, string(shardContent))
	lines := make([]string, 0, len(records))
	for _, record := range records {
		metadata := record["metadata"].(map[string]any)
		metadata["last_ingest_at"] = "2026-06-30T14:15:00Z"
		metadata["merged_prs_since_last_ingest"] = float64(7)
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(encoded))
	}
	writeFileForTest(t, shardPath, strings.Join(lines, "\n")+"\n")

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if got := stringFromAny(report.Metadata["source_artifact_digest"]); got != savedDigest {
		t.Fatalf("source digest = %q, want unchanged %q", got, savedDigest)
	}
	if report.Baseline.Status != "current" || report.Baseline.Stale {
		t.Fatalf("baseline = %#v, want current after freshness-only metadata change", report.Baseline)
	}
}

func TestBacktestRollsBackBaselineWhenReportWriteFails(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	baselinePath := filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json")
	originalBaseline := `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.01,
  "metadata": {
    "source_artifact_digest": "sha256:accepted"
  }
}
`
	writeFileForTest(t, baselinePath, originalBaseline)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "reports"), "not a directory\n")

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d", "--save-baseline"}, false)

	if code == ExitSuccess {
		t.Fatalf("backtest unexpectedly succeeded, stderr = %q, stdout = %q", stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "backtest report directory") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	baselineContent, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(baselineContent) != originalBaseline {
		t.Fatalf("baseline was not rolled back after report write failure:\n%s", baselineContent)
	}
}

func TestBacktestRemovesJSONReportWhenHTMLWriteFails(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	probeStdout, probeStderr, probeCode := runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if probeCode != ExitSuccess {
		t.Fatalf("probe backtest exit code = %d, stderr = %q, stdout = %q", probeCode, probeStderr, probeStdout)
	}
	probeResult := decodeResult(t, probeStdout)
	jsonRel, _ := probeResult.Data["json_report_path"].(string)
	htmlRel, _ := probeResult.Data["html_report_path"].(string)
	jsonPath := filepath.Join(tempDir, filepath.FromSlash(jsonRel))
	htmlPath := filepath.Join(tempDir, filepath.FromSlash(htmlRel))
	if err := os.Remove(jsonPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(htmlPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(htmlPath, 0o755); err != nil {
		t.Fatal(err)
	}
	baselinePath := filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json")
	originalBaseline := `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.01,
  "metadata": {
    "source_artifact_digest": "sha256:accepted"
  }
}
`
	writeFileForTest(t, baselinePath, originalBaseline)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d", "--save-baseline"}, false)

	if code == ExitSuccess {
		t.Fatalf("backtest unexpectedly succeeded, stderr = %q, stdout = %q", stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "HTML recurrence report") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Fatalf("partial JSON report still exists after HTML failure: %v", err)
	}
	baselineContent, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(baselineContent) != originalBaseline {
		t.Fatalf("baseline was not rolled back after HTML report failure:\n%s", baselineContent)
	}
}

func TestBacktestPreservesExistingJSONReportWhenHTMLWriteFails(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	probeStdout, probeStderr, probeCode := runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if probeCode != ExitSuccess {
		t.Fatalf("probe backtest exit code = %d, stderr = %q, stdout = %q", probeCode, probeStderr, probeStdout)
	}
	probeResult := decodeResult(t, probeStdout)
	jsonRel, _ := probeResult.Data["json_report_path"].(string)
	htmlRel, _ := probeResult.Data["html_report_path"].(string)
	jsonPath := filepath.Join(tempDir, filepath.FromSlash(jsonRel))
	htmlPath := filepath.Join(tempDir, filepath.FromSlash(htmlRel))
	originalJSON := []byte("{\"object_type\":\"prior.recurrence_report\"}\n")
	if err := os.WriteFile(jsonPath, originalJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(htmlPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(htmlPath, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code == ExitSuccess {
		t.Fatalf("backtest unexpectedly succeeded, stderr = %q, stdout = %q", stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "HTML recurrence report") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	content, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, originalJSON) {
		t.Fatalf("existing JSON report changed after HTML failure:\n%s", content)
	}
}

func TestBacktestFailsClosedWhenBaselinePathIsDirectory(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	baselinePath := filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json")
	if err := os.MkdirAll(baselinePath, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d", "--save-baseline"}, false)

	if code == ExitSuccess {
		t.Fatalf("backtest unexpectedly succeeded, stderr = %q, stdout = %q", stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "ERR baseline") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	info, err := os.Stat(baselinePath)
	if err != nil || !info.IsDir() {
		t.Fatalf("baseline path = info:%#v err:%v, want directory left intact", info, err)
	}
}

func TestBacktestFailsClosedForNonRedactedExperienceShard(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `"redaction_status":"applied"`, `"redaction_status":"not_applicable"`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "redaction_safety_failed" || !strings.Contains(result.Errors[0].Message, "redaction_status") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if result.RedactionStatus != "failed_closed" {
		t.Fatalf("redaction_status = %q, want failed_closed", result.RedactionStatus)
	}
}

func TestBacktestFailsClosedForNonPrivateExperienceShard(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `"share_scope":"private"`, `"share_scope":"org"`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "redaction_safety_failed" || !strings.Contains(result.Errors[0].Message, "share_scope") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if result.RedactionStatus != "failed_closed" {
		t.Fatalf("redaction_status = %q, want failed_closed", result.RedactionStatus)
	}
}

func TestBacktestRejectsNonRepoRelativeExperiencePaths(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `"paths":["packages/billing/invoice.py"]`, `"paths":["../secrets.txt"]`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitValidation {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "artifact_contract_validation_failed" || !strings.Contains(result.Errors[0].Message, "repo-relative") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestBacktestRejectsNonCanonicalExperienceProvenanceURLs(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `https://github.com/acme/billing-service/pull/101`, `https://github.com/acme/billing-service/pull/101?from=shard`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "provenance_integrity_failed" || !strings.Contains(result.Errors[0].Message, "canonical") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestBacktestRejectsExperienceProvenanceWithoutMatchingPRURL(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `https://github.com/acme/billing-service/pull/101`, `https://github.com/acme/billing-service/pull/102`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "provenance_integrity_failed" || !strings.Contains(result.Errors[0].Message, "action.pr") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestBacktestAcceptsCanonicalNonPRProvenanceAndDerivesCitation(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","check_run_url":"https://github.com/acme/billing-service/actions/runs/991"}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","check_run_url":"https://github.com/acme/billing-service/actions/runs/992"}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if len(report.Citations) != 2 ||
		report.Citations[0].URL != "https://github.com/acme/billing-service/pull/101" ||
		report.Citations[1].URL != "https://github.com/acme/billing-service/pull/102" {
		t.Fatalf("citations = %#v, want derived PR citations for non-PR provenance", report.Citations)
	}
}

func TestBacktestRejectsExperienceProvenanceFromDifferentRepo(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `https://github.com/acme/billing-service/pull/101`, `https://github.com/other-org/other-repo/actions/runs/991`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "provenance_integrity_failed" || !strings.Contains(result.Errors[0].Message, "repo") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestBacktestRejectsPullSubpageProvenanceForDifferentPR(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `https://github.com/acme/billing-service/pull/101`, `https://github.com/acme/billing-service/pull/102/files`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "provenance_integrity_failed" || !strings.Contains(result.Errors[0].Message, "action.pr") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestBacktestPairsCurrentWithAnyEarlierConfirmedRecurrence(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/tax.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
		`{"experience_id":"exp_0003","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-20T10:00:00Z","pr":103,"commit":"abc003","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/103"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.Summary.PossibleRecurrenceCount != 1 {
		t.Fatalf("recurrence counts confirmed=%d possible=%d, report=%#v", report.Summary.ConfirmedRecurrenceCount, report.Summary.PossibleRecurrenceCount, report)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.PriorExperienceID != "exp_0001" || confirmed.CurrentExperienceID != "exp_0003" {
		t.Fatalf("confirmed pair = %#v, want current exp_0003 paired with earlier overlapping exp_0001", confirmed)
	}
	possible := report.PossibleRecurrences[0]
	if possible.PriorExperienceID != "exp_0001" || possible.CurrentExperienceID != "exp_0002" {
		t.Fatalf("possible pair = %#v, want disjoint middle occurrence reported separately", possible)
	}
	if report.HeadlineERR != 0.3333 {
		t.Fatalf("headline_err = %.4f, want confirmed-only 1/3", report.HeadlineERR)
	}
}

func TestBacktestIndexesHumanFailuresAsPriorRecurrenceEvidence(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_human_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"human","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_agent_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.Summary.HumanFailureExcludedCount != 1 || report.Summary.AgentFailureDenominator != 1 {
		t.Fatalf("summary = %#v, want one human prior excluded and one agent denominator", report.Summary)
	}
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.HeadlineERR != 1 {
		t.Fatalf("confirmed=%d headline=%.4f, want human-prior recurrence over one agent failure", report.Summary.ConfirmedRecurrenceCount, report.HeadlineERR)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.PriorExperienceID != "exp_human_0001" || confirmed.CurrentExperienceID != "exp_agent_0002" {
		t.Fatalf("confirmed pair = %#v, want agent failure repeating prior human failure", confirmed)
	}
}

func TestBacktestSkipsFlakeDiscountedHumanPriors(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_human_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"human","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","flake_discount":1.0,"provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_agent_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.Summary.HumanFailureExcludedCount != 1 || report.Summary.AgentFailureDenominator != 1 {
		t.Fatalf("summary = %#v, want one excluded human failure and one agent denominator", report.Summary)
	}
	if report.Summary.ConfirmedRecurrenceCount != 0 || report.HeadlineERR != 0 {
		t.Fatalf("confirmed=%d headline=%.4f, want flaky human prior skipped", report.Summary.ConfirmedRecurrenceCount, report.HeadlineERR)
	}
}

func TestBacktestDoesNotPairSamePRAsRecurrence(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze_a","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-02T10:00:00Z","pr":101,"commit":"abc001-rerun","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze_a","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0003","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze_a","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.Summary.PossibleRecurrenceCount != 0 {
		t.Fatalf("recurrence counts confirmed=%d possible=%d, report=%#v", report.Summary.ConfirmedRecurrenceCount, report.Summary.PossibleRecurrenceCount, report)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.CurrentPR == confirmed.PriorPR {
		t.Fatalf("confirmed pair = %#v, same PR should not count as recurrence", confirmed)
	}
	if confirmed.CurrentExperienceID != "exp_0003" || confirmed.PriorPR != 101 {
		t.Fatalf("confirmed pair = %#v, want later PR paired against prior different PR", confirmed)
	}
}

func TestBacktestGroupsEquivalentCanonicalSignatureFields(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_pytest","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_go_test","signature_class":"test_failure","check_name":"go-test-rerun","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.HeadlineERR != 0.5 {
		t.Fatalf("confirmed=%d headline=%.4f, report=%#v", report.Summary.ConfirmedRecurrenceCount, report.HeadlineERR, report)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.PriorExperienceID != "exp_0001" || confirmed.CurrentExperienceID != "exp_0002" {
		t.Fatalf("confirmed pair = %#v, want canonical class/key grouping despite different generated ids", confirmed)
	}
}

func TestBacktestTopRepeatedMistakesAggregateByMatchedSignature(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_clock","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_clock","signature_class":"test_failure","check_name":"go-test-rerun","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
		`{"experience_id":"exp_0003","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-20T10:00:00Z","pr":103,"commit":"abc003","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_rspec","signature_class":"test_failure","check_name":"rspec-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/103"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.Summary.ConfirmedRecurrenceCount != 2 {
		t.Fatalf("confirmed recurrences = %d, report=%#v", report.Summary.ConfirmedRecurrenceCount, report)
	}
	if len(report.TopRepeatedMistakes) != 1 {
		t.Fatalf("top repeated mistakes = %#v, want one matched-signature aggregate", report.TopRepeatedMistakes)
	}
	mistake := report.TopRepeatedMistakes[0]
	if mistake.SignatureID != "class_key:test_failure:tests/billing/test_invoice.py::test_clock" || mistake.RepeatCount != 2 {
		t.Fatalf("top repeated mistake = %#v, want matched class/key count 2", mistake)
	}
	if len(mistake.PRs) != 3 || mistake.PRs[0] != 101 || mistake.PRs[1] != 102 || mistake.PRs[2] != 103 {
		t.Fatalf("top repeated mistake PRs = %#v, want all matched PRs", mistake.PRs)
	}
	if !stringSlicesEqual(mistake.ExperienceIDs, []string{"exp_0001", "exp_0002", "exp_0003"}) {
		t.Fatalf("top repeated mistake experiences = %#v", mistake.ExperienceIDs)
	}
}

func TestBacktestGroupsClassKeyEvenWithDifferentMessageFingerprints(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","message":"expected 2026-01-01 but got 2025-12-31","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","message":"expected 2026-01-10 but got 2026-01-09","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.HeadlineERR != 0.5 {
		t.Fatalf("confirmed=%d headline=%.4f, report=%#v", report.Summary.ConfirmedRecurrenceCount, report.HeadlineERR, report)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.PriorExperienceID != "exp_0001" || confirmed.CurrentExperienceID != "exp_0002" {
		t.Fatalf("confirmed pair = %#v, want class/key recurrence despite different message fingerprints", confirmed)
	}
}

func TestBacktestGroupsEquivalentMessageFingerprint(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_invoice","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","message":"expected 2026-01-01 but got local timezone drift","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_invoice_rerun","signature_class":"runtime_failure","check_name":"pytest-billing","signature_key":"runtime-clock-drift","extraction_confidence":"structured","message":"expected 2026-01-01 but got local timezone drift","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.HeadlineERR != 0.5 {
		t.Fatalf("confirmed=%d headline=%.4f, report=%#v", report.Summary.ConfirmedRecurrenceCount, report.HeadlineERR, report)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.PriorExperienceID != "exp_0001" || confirmed.CurrentExperienceID != "exp_0002" {
		t.Fatalf("confirmed pair = %#v, want message-fingerprint grouping despite different generated ids and classes", confirmed)
	}
}

func TestBacktestExplicitEnabledGateEvaluatesThreshold(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), `gate:
  enabled: false`, `gate:
  enabled: true
  max_error_recurrence_rate: 0.0`)
	baselinePath := filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json")
	originalBaseline := `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.01,
  "metadata": {
    "source_artifact_digest": "sha256:accepted"
  }
}
`
	writeFileForTest(t, baselinePath, originalBaseline)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d", "--save-baseline"}, false)
	if code != ExitGate {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "recurrence_gate_failed" {
		t.Fatalf("errors = %#v", result.Errors)
	}
	report := decodeBacktestReportFromResult(t, result)
	if !report.Gate.Enabled || report.Gate.Status != "fail" || report.Gate.Threshold == nil || *report.Gate.Threshold != 0 {
		t.Fatalf("gate = %#v, want enabled failing threshold", report.Gate)
	}
	gateJSON, err := json.Marshal(report.Gate)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(gateJSON, []byte(`"threshold":0`)) {
		t.Fatalf("gate JSON = %s, want zero threshold preserved", gateJSON)
	}
	baselineContent, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(baselineContent) != originalBaseline {
		t.Fatalf("baseline was overwritten by failing gate:\n%s", baselineContent)
	}
}

func TestBacktestRepeatedRunsUseStableReportIDAndArtifacts(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	firstStdout, firstStderr, firstCode := runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	secondStdout, secondStderr, secondCode := runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if firstCode != ExitSuccess || secondCode != ExitSuccess {
		t.Fatalf("backtest codes = %d/%d stderr=%q/%q stdout=%q/%q", firstCode, secondCode, firstStderr, secondStderr, firstStdout, secondStdout)
	}
	firstResult := decodeResult(t, firstStdout)
	secondResult := decodeResult(t, secondStdout)
	firstReport := decodeBacktestReportFromResult(t, firstResult)
	secondReport := decodeBacktestReportFromResult(t, secondResult)
	if firstReport.ReportID != secondReport.ReportID {
		t.Fatalf("report_id changed across repeated runs: %q then %q", firstReport.ReportID, secondReport.ReportID)
	}
	firstPath, _ := firstResult.Data["json_report_path"].(string)
	secondPath, _ := secondResult.Data["json_report_path"].(string)
	if firstPath != secondPath {
		t.Fatalf("json report path changed: %q then %q", firstPath, secondPath)
	}
	firstContent, err := os.ReadFile(filepath.Join(tempDir, filepath.FromSlash(firstPath)))
	if err != nil {
		t.Fatal(err)
	}
	secondContent, err := os.ReadFile(filepath.Join(tempDir, filepath.FromSlash(secondPath)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstContent, secondContent) {
		t.Fatalf("json report content changed across repeated runs:\nfirst=%s\nsecond=%s", firstContent, secondContent)
	}
}

func TestDistillDraftsDeterministicCandidateRulesReviewAndMemoryPage(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	writeFileForTest(t, filepath.Join(tempDir, "packages", "search", "query.py"), "def query():\n    return []\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0101","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":101,"commit":"abc101","paths":["packages/billing/invoice.py","tests/billing/test_invoice.py"],"actor_kind":"agent","attribution_method":"coauthor_trailer","attribution_confidence":0.91,"outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_clock","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0102","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-15T10:00:00Z","pr":102,"commit":"abc102","paths":["packages/billing/invoice.py","tests/billing/test_invoice.py"],"actor_kind":"agent","attribution_method":"coauthor_trailer","attribution_confidence":0.91,"outcome_kind":"revert","terminal_state":"reverted","signature_id":"sig_billing_clock","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"log_parsed_high","flake_discount":0.25,"provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
		`{"experience_id":"exp_0110","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-20T10:00:00Z","pr":110,"commit":"abc110","paths":["packages/search/query.py","tests/search/test_query.py"],"actor_kind":"agent","attribution_method":"pr_label","attribution_confidence":0.9,"outcome_kind":"fix_held","terminal_state":"held","signature_id":"sig_search_escape","signature_class":"test_failure","check_name":"pytest-search","signature_key":"tests/search/test_query.py::test_escape","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/110"]}`,
		`{"experience_id":"exp_0111","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-22T10:00:00Z","pr":111,"commit":"abc111","paths":["packages/search/query.py","tests/search/test_query.py"],"actor_kind":"agent","attribution_method":"pr_label","attribution_confidence":0.9,"outcome_kind":"merged_clean","terminal_state":"passed","signature_id":"sig_search_escape","signature_class":"test_failure","check_name":"pytest-search","signature_key":"tests/search/test_query.py::test_escape","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/111"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "distill" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	if result.RedactionStatus != "applied" {
		t.Fatalf("redaction_status = %q", result.RedactionStatus)
	}
	if got := int(result.Data["rules_written"].(float64)); got != 2 {
		t.Fatalf("rules_written = %d", got)
	}
	if got := int(result.Data["active_rules"].(float64)); got != 0 {
		t.Fatalf("active_rules = %d, want review gate to keep drafts inactive", got)
	}
	rules := loadRuleDocsByKindForTest(t, tempDir)
	avoid := rules["avoid"]
	playbook := rules["playbook"]
	if avoid.Scalars["status"].Value != "candidate" || avoid.Scalars["review.label"].Value != "suggested" {
		t.Fatalf("avoid lifecycle = status %q review %q", avoid.Scalars["status"].Value, avoid.Scalars["review.label"].Value)
	}
	if playbook.Scalars["status"].Value != "candidate" || playbook.Scalars["review.label"].Value != "suggested" {
		t.Fatalf("playbook lifecycle = status %q review %q", playbook.Scalars["status"].Value, playbook.Scalars["review.label"].Value)
	}
	if avoid.Scalars["review.statement_origin"].Value != "cluster_summary" || playbook.Scalars["review.statement_origin"].Value != "cluster_summary" {
		t.Fatalf("statement origins = avoid %q playbook %q", avoid.Scalars["review.statement_origin"].Value, playbook.Scalars["review.statement_origin"].Value)
	}
	if avoid.Scalars["metadata.confidence_inputs.drafting_model_weight"].Value != "0" {
		t.Fatalf("drafting model affected confidence: %#v", avoid.Scalars["metadata.confidence_inputs.drafting_model_weight"])
	}
	if avoid.Scalars["metadata.confidence_inputs.evidence_count"].Value != "2" ||
		avoid.Scalars["metadata.confidence_inputs.contradictions"].Value != "0" ||
		avoid.Scalars["metadata.confidence_inputs.flake_discount"].Value == "0" ||
		avoid.Scalars["metadata.decay.half_life_days"].Value != "90" {
		t.Fatalf("avoid confidence metadata = %#v", avoid.Scalars)
	}
	if avoid.Scalars["metadata.memory_source"].Value != "verified_outcome_events" ||
		avoid.Scalars["metadata.source_record_type"].Value != "relia.experience_record" {
		t.Fatalf("avoid memory source metadata = %#v", avoid.Scalars)
	}
	if got := yamlScalarValuesForTest(avoid.Lists["metadata.excluded_memory_sources"]); !stringSlicesEqual(got, []string{"agent_self_report", "agent_reflection"}) {
		t.Fatalf("excluded memory sources = %#v", got)
	}
	if !assessmentRuleHasPositivePlaybookEvidence(playbook) {
		t.Fatalf("playbook rule did not cite held or clean evidence: %#v", playbook.ListMaps["provenance"])
	}
	firstAvoidContent := readRuleByIDForTest(t, tempDir, avoid.Scalars["id"].Value)
	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("second distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	secondAvoidContent := readRuleByIDForTest(t, tempDir, avoid.Scalars["id"].Value)
	if firstAvoidContent != secondAvoidContent {
		t.Fatalf("distill was not deterministic:\nfirst=%s\nsecond=%s", firstAvoidContent, secondAvoidContent)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "review", "--rule", avoid.Scalars["id"].Value, "--label", "accepted"}, false)
	if code != ExitSuccess {
		t.Fatalf("review exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	reviewed := parseRuleDocForTest(t, readRuleByIDForTest(t, tempDir, avoid.Scalars["id"].Value))
	if reviewed.Scalars["status"].Value != "active" || reviewed.Scalars["review.label"].Value != "accepted" {
		t.Fatalf("reviewed lifecycle = status %q review %q", reviewed.Scalars["status"].Value, reviewed.Scalars["review.label"].Value)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "memory", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("memory exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	page, err := os.ReadFile(filepath.Join(tempDir, "memory", "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Relia Memory",
		"active",
		"candidate",
		"confidence",
		"[PR #101](https://github.com/acme/billing-service/pull/101)",
		"[PR #110](https://github.com/acme/billing-service/pull/110)",
	} {
		if !strings.Contains(string(page), want) {
			t.Fatalf("MEMORY.md missing %q:\n%s", want, page)
		}
	}
	for _, want := range []string{
		"## Strong Memory",
		"## Weak Memory",
		"Active accepted rules",
		"Candidate, stale, contradicted, and retired",
	} {
		if !strings.Contains(string(page), want) {
			t.Fatalf("MEMORY.md missing weak/strong separation marker %q:\n%s", want, page)
		}
	}
}

func TestDistillInputDraftsAvoidRuleFromPlantedRecurrenceCluster(t *testing.T) {
	sourceRoot := findRepoRootForTest(t)
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	fixtureRel := filepath.Join("examples", "demo", "seeded-repo", "outcomes.jsonl")
	fixtureContent, err := os.ReadFile(filepath.Join(sourceRoot, fixtureRel))
	if err != nil {
		t.Fatal(err)
	}
	writeFileForTest(t, filepath.Join(tempDir, fixtureRel), string(fixtureContent))
	for _, record := range decodeJSONLines(t, string(fixtureContent)) {
		for _, path := range stringListField(record, "paths", "context.paths") {
			clean, ok := cleanRepoPath(path)
			if !ok {
				t.Fatalf("fixture path is not repo-relative: %q", path)
			}
			writeFileForTest(t, filepath.Join(tempDir, filepath.FromSlash(filepath.ToSlash(clean))), "fixture path\n")
		}
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", filepath.ToSlash(fixtureRel), "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("distill input exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got, _ := result.Data["input_path"].(string); got != filepath.ToSlash(fixtureRel) {
		t.Fatalf("input_path = %q, want %q", got, filepath.ToSlash(fixtureRel))
	}
	if got := int(result.Data["rules_written"].(float64)); got < 1 {
		t.Fatalf("rules_written = %d, want at least one rule from planted fixture", got)
	}
	wantExperienceIDs := []string{"exp_0142", "exp_0187", "exp_0203"}
	avoid := findRuleDocByEvidenceForTest(t, tempDir, "avoid", wantExperienceIDs)
	if avoid.Scalars["status"].Value != "candidate" ||
		avoid.Scalars["review.label"].Value != "suggested" ||
		avoid.Scalars["evidence.count"].Value != "3" ||
		avoid.Scalars["evidence.contradictions"].Value != "0" {
		t.Fatalf("avoid rule lifecycle/evidence = %#v", avoid.Scalars)
	}
	if got := yamlScalarValuesForTest(avoid.Lists["evidence.experiences"]); !stringSlicesEqual(got, wantExperienceIDs) {
		t.Fatalf("avoid evidence experiences = %#v, want %#v", got, wantExperienceIDs)
	}
	wantCitations := map[int]string{
		142: "https://github.com/Clyra-AI/relia-demo-seed/pull/142",
		187: "https://github.com/Clyra-AI/relia-demo-seed/pull/187",
		203: "https://github.com/Clyra-AI/relia-demo-seed/pull/203",
	}
	for _, entry := range avoid.ListMaps["provenance"] {
		pr, err := strconv.Atoi(entry["pr"].Value)
		if err != nil {
			t.Fatalf("provenance pr = %#v", entry["pr"])
		}
		if wantCitations[pr] != entry["url"].Value {
			t.Fatalf("provenance entry = %#v, want PR citation %q", entry, wantCitations[pr])
		}
		delete(wantCitations, pr)
	}
	if len(wantCitations) != 0 {
		t.Fatalf("missing planted recurrence citations: %#v", wantCitations)
	}
}

func TestDistillRejectsCanonicalSelfReportBeforeMemoryWrite(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "metadata_source_kind",
			mutate: func(record map[string]any) {
				metadata := record["metadata"].(map[string]any)
				metadata["source_kind"] = "agent_reflection"
			},
		},
		{
			name: "metadata_event_type",
			mutate: func(record map[string]any) {
				metadata := record["metadata"].(map[string]any)
				metadata["event_type"] = "agent_reflection"
			},
		},
		{
			name: "metadata_source_object_type",
			mutate: func(record map[string]any) {
				metadata := record["metadata"].(map[string]any)
				metadata["source"] = map[string]any{"object_type": "agent_self_report"}
			},
		},
		{
			name: "camel_case_metadata_source_kind",
			mutate: func(record map[string]any) {
				metadata := record["metadata"].(map[string]any)
				metadata["source_kind"] = "agentSelfReport"
			},
		},
		{
			name: "top_level_event_type",
			mutate: func(record map[string]any) {
				record["event_type"] = "agent_reflection"
			},
		},
		{
			name: "top_level_source_kind",
			mutate: func(record map[string]any) {
				record["source_kind"] = "agent_self_report"
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := setupContractRepo(t)
			t.Chdir(tempDir)
			writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
			inputRel := filepath.ToSlash(filepath.Join("fixtures", "self-report-experience-records.jsonl"))
			inputPath := filepath.Join(tempDir, filepath.FromSlash(inputRel))
			record := canonicalExperienceRecordMapForTest("exp_self_report_001", 521)
			tc.mutate(record)
			content, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			writeFileForTest(t, inputPath, string(content)+"\n")

			stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)

			if code != ExitValidation {
				t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if result.Errors[0].Type != "artifact_contract_validation_failed" ||
				!strings.Contains(result.Errors[0].Message, "self-reports") {
				t.Fatalf("errors = %#v", result.Errors)
			}
			matches, err := filepath.Glob(filepath.Join(tempDir, "memory", "rules", "*.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 0 {
				t.Fatalf("distill wrote memory rules from an agent self-report: %#v", matches)
			}
		})
	}
}

func TestDistillSeparatesCanonicalSignatureClustersByCheckName(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-canonical-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0501","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":501,"commit":"abc501","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_a","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/501"]}`,
		`{"experience_id":"exp_0502","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-08T10:00:00Z","pr":502,"commit":"abc502","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_b","signature_class":"test_failure","check_name":"pytest-billing-rerun","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/502"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got := int(result.Data["rules_written"].(float64)); got != 2 {
		t.Fatalf("rules_written = %d, want separate class/check/key clusters", got)
	}
	wantEvidence := map[string]bool{"exp_0501": false, "exp_0502": false}
	for _, rule := range loadRuleDocsForTest(t, tempDir) {
		if rule.Scalars["kind"].Value != "avoid" {
			continue
		}
		if rule.Scalars["evidence.count"].Value != "1" {
			t.Fatalf("evidence.count = %q, want one record per class/check/key cluster", rule.Scalars["evidence.count"].Value)
		}
		experiences := yamlScalarValuesForTest(rule.Lists["evidence.experiences"])
		if len(experiences) != 1 {
			t.Fatalf("evidence experiences = %#v, want one record per separated check", experiences)
		}
		if _, ok := wantEvidence[experiences[0]]; !ok {
			t.Fatalf("unexpected evidence experience %q", experiences[0])
		}
		wantEvidence[experiences[0]] = true
		confidence, err := strconv.ParseFloat(rule.Scalars["confidence"].Value, 64)
		if err != nil {
			t.Fatal(err)
		}
		if confidence > 0.6 {
			t.Fatalf("confidence = %.4f, want capped at 0.6 until three confirmed experiences", confidence)
		}
		if rule.Scalars["metadata.embedding_mode"].Value != "signature" ||
			rule.Scalars["metadata.cluster.provenance"].Value != "signature_only" {
			t.Fatalf("signature fallback provenance metadata = %#v", rule.Scalars)
		}
	}
	for experienceID, seen := range wantEvidence {
		if !seen {
			t.Fatalf("missing separated avoid rule for %s", experienceID)
		}
	}
}

func TestDistillInputPreservesCanonicalExperienceSignatureMetadata(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	writeFileForTest(t, filepath.Join(tempDir, "packages", "payments", "clock.py"), "def now():\n    return 1\n")
	inputRel := filepath.ToSlash(filepath.Join("fixtures", "distill-canonical-experience-records.jsonl"))
	inputPath := filepath.Join(tempDir, filepath.FromSlash(inputRel))
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"object_type":"relia.experience_record","schema_version":"1.0","experience_id":"exp_0521","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","attribution":{"actor_kind":"agent","method":"manual","confidence":1},"context":{"paths":["packages/billing/invoice.py"],"diff_fingerprint":"sha256:canonical-a"},"action":{"pr":521,"commit":"abc521"},"outcome":{"kind":"ci_failure","terminal_state":"failed","signature":{"signature_id":"sig_canonical_a","extraction_confidence":"structured"}},"provenance":{"urls":["https://github.com/acme/billing-service/pull/521"]},"flake_discount":0,"org_eligible":false,"share_scope":"private","redaction_status":"applied","metadata":{"signature":{"class":"test_failure","check_name":"pytest-billing","key":"tests/billing/test_invoice.py::test_clock","message_fingerprint":"sha256:canonical-shared-input","extraction_method":"structured"},"source_kind":"ingest"}}`,
		`{"object_type":"relia.experience_record","schema_version":"1.0","experience_id":"exp_0522","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-08T10:00:00Z","attribution":{"actor_kind":"agent","method":"manual","confidence":1},"context":{"paths":["packages/payments/clock.py"],"diff_fingerprint":"sha256:canonical-b"},"action":{"pr":522,"commit":"abc522"},"outcome":{"kind":"ci_failure","terminal_state":"failed","signature":{"signature_id":"sig_canonical_b","extraction_confidence":"structured"}},"provenance":{"urls":["https://github.com/acme/billing-service/pull/522"]},"flake_discount":0,"org_eligible":false,"share_scope":"private","redaction_status":"applied","metadata":{"signature":{"class":"test_failure","check_name":"go-test-payments","key":"tests/payments/test_clock.py::test_clock","message_fingerprint":"sha256:canonical-shared-input","extraction_method":"structured"},"source_kind":"ingest"}}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill input exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got := int(result.Data["rules_written"].(float64)); got != 1 {
		t.Fatalf("rules_written = %d, want one canonical message-fingerprint cluster", got)
	}
	avoid := loadRuleDocsByKindForTest(t, tempDir)["avoid"]
	if got := yamlScalarValuesForTest(avoid.Lists["evidence.experiences"]); !stringSlicesEqual(got, []string{"exp_0521", "exp_0522"}) {
		t.Fatalf("avoid evidence experiences = %#v, want canonical input message-fingerprint cluster", got)
	}
	if got := avoid.Scalars["metadata.cluster.key"].Value; got != "message|sha256:canonical-shared-input" {
		t.Fatalf("metadata.cluster.key = %q, want canonical message fingerprint key", got)
	}
}

func TestDistillInputRejectsOrgEligibleCanonicalExperience(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputRel := filepath.ToSlash(filepath.Join("fixtures", "distill-org-eligible-canonical-experience.jsonl"))
	inputPath := filepath.Join(tempDir, filepath.FromSlash(inputRel))
	writeFileForTest(t, inputPath, `{"object_type":"relia.experience_record","schema_version":"1.0","experience_id":"exp_0523","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","attribution":{"actor_kind":"agent","method":"manual","confidence":1},"context":{"paths":["packages/billing/invoice.py"],"diff_fingerprint":"sha256:canonical-org"},"action":{"pr":523,"commit":"abc523"},"outcome":{"kind":"ci_failure","terminal_state":"failed","signature":{"signature_id":"sig_canonical_org","extraction_confidence":"structured"}},"provenance":{"urls":["https://github.com/acme/billing-service/pull/523"]},"flake_discount":0,"org_eligible":true,"share_scope":"private","redaction_status":"applied","metadata":{"signature":{"class":"test_failure","check_name":"pytest-billing","key":"tests/billing/test_invoice.py::test_clock","message_fingerprint":"sha256:canonical-org","extraction_method":"structured"},"source_kind":"ingest"}}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)
	if code != ExitValidation {
		t.Fatalf("distill org-eligible input exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "org_eligible must be false") {
		t.Fatalf("distill org-eligible errors = %#v", result.Errors)
	}
}

func TestDistillInputRejectsIncompleteCanonicalExperience(t *testing.T) {
	tests := []struct {
		name        string
		removePath  []string
		wantMessage string
	}{
		{name: "commit", removePath: []string{"action", "commit"}, wantMessage: "action.commit must be provided"},
		{name: "method", removePath: []string{"attribution", "method"}, wantMessage: "attribution.method must be provided"},
		{name: "confidence", removePath: []string{"attribution", "confidence"}, wantMessage: "attribution.confidence must be provided"},
		{name: "diff", removePath: []string{"context", "diff_fingerprint"}, wantMessage: "context.diff_fingerprint must be provided"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := setupContractRepo(t)
			t.Chdir(tempDir)
			inputRel := filepath.ToSlash(filepath.Join("fixtures", "distill-incomplete-canonical-experience.jsonl"))
			inputPath := filepath.Join(tempDir, filepath.FromSlash(inputRel))
			record := canonicalExperienceRecordMapForTest("exp_0524", 524)
			deleteNestedMapFieldForTest(record, tc.removePath...)
			content, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			writeFileForTest(t, inputPath, string(content)+"\n")

			stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)
			if code != ExitValidation {
				t.Fatalf("distill incomplete input exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, tc.wantMessage) {
				t.Fatalf("distill incomplete errors = %#v, want %q", result.Errors, tc.wantMessage)
			}
		})
	}
}

func canonicalExperienceRecordMapForTest(experienceID string, pr int) map[string]any {
	return map[string]any{
		"object_type":    "relia.experience_record",
		"schema_version": commandSchemaVersion,
		"experience_id":  experienceID,
		"repo": map[string]any{
			"provider": "github",
			"owner":    "acme",
			"name":     "billing-service",
		},
		"recorded_at": "2026-04-01T10:00:00Z",
		"attribution": map[string]any{
			"actor_kind": "agent",
			"method":     "manual",
			"confidence": 1,
		},
		"context": map[string]any{
			"paths":            []any{"packages/billing/invoice.py"},
			"diff_fingerprint": "sha256:canonical-complete",
		},
		"action": map[string]any{
			"pr":     pr,
			"commit": "abc524",
		},
		"outcome": map[string]any{
			"kind":           "ci_failure",
			"terminal_state": "failed",
			"signature": map[string]any{
				"signature_id":          "sig_canonical_complete",
				"extraction_confidence": "structured",
			},
		},
		"provenance": map[string]any{
			"urls": []any{fmt.Sprintf("https://github.com/acme/billing-service/pull/%d", pr)},
		},
		"flake_discount":   0,
		"org_eligible":     false,
		"share_scope":      "private",
		"redaction_status": "applied",
		"metadata": map[string]any{
			"signature": map[string]any{
				"class":               "test_failure",
				"check_name":          "pytest-billing",
				"key":                 "tests/billing/test_invoice.py::test_clock",
				"message_fingerprint": "sha256:canonical-complete",
				"extraction_method":   "structured",
			},
			"source_kind": "ingest",
		},
	}
}

func deleteNestedMapFieldForTest(root map[string]any, path ...string) {
	current := root
	for _, key := range path[:len(path)-1] {
		next, _ := current[key].(map[string]any)
		current = next
	}
	delete(current, path[len(path)-1])
}

func TestDistillStableIDCheckKeyClustersPositiveEvidence(t *testing.T) {
	failure := distillClusterKeyForTest("ci_failure", "sig_time_freeze", "test_failure", "pytest-billing", "tests/billing/test_invoice_time.py::test_rollover_uses_frozen_clock")
	revert := distillClusterKeyForTest("revert", "sig_time_freeze", "revert", "pytest-billing", "tests/billing/test_invoice_time.py::test_rollover_uses_frozen_clock")
	held := distillClusterKeyForTest("fix_held", "sig_time_freeze", "held_fix", "pytest-billing", "tests/billing/test_invoice_time.py::test_rollover_uses_frozen_clock")
	if failure == "" || revert == "" {
		t.Fatalf("failure key = %q revert key = %q, want non-empty keys", failure, revert)
	}
	if failure != revert {
		t.Fatalf("failure key = %q revert key = %q, want stable signature ID/check/key to co-cluster related outcomes", failure, revert)
	}
	if held != failure {
		t.Fatalf("held fix key = %q failure key = %q, want stable ID/check/key to keep positive evidence attached to the same distill cluster", held, failure)
	}
}

func TestDistillSeparatesReusedStableSignatureIDsByCheckName(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-reused-stable-id-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0551","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":551,"commit":"abc551","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_reused_monorepo","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/551"]}`,
		`{"experience_id":"exp_0552","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-08T10:00:00Z","pr":552,"commit":"abc552","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_reused_monorepo","signature_class":"test_failure","check_name":"go-test-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/552"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got := int(result.Data["rules_written"].(float64)); got != 2 {
		t.Fatalf("rules_written = %d, want reused stable signature ID separated by check_name", got)
	}
}

func TestDistillClustersMatchingMessageFingerprintsAcrossChecks(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-message-fingerprint-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0601","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":601,"commit":"abc601","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","message_fingerprint":"sha256:shared-clock-failure","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/601"]}`,
		`{"experience_id":"exp_0602","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-08T10:00:00Z","pr":602,"commit":"abc602","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_class":"test_failure","check_name":"go-test-billing","signature_key":"tests/billing/test_invoice.py::test_clock","message_fingerprint":"sha256:shared-clock-failure","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/602"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got := int(result.Data["rules_written"].(float64)); got != 1 {
		t.Fatalf("rules_written = %d, want one message-fingerprint cluster", got)
	}
	avoid := loadRuleDocsByKindForTest(t, tempDir)["avoid"]
	if got := yamlScalarValuesForTest(avoid.Lists["evidence.experiences"]); !stringSlicesEqual(got, []string{"exp_0601", "exp_0602"}) {
		t.Fatalf("avoid evidence experiences = %#v, want message-fingerprint cluster", got)
	}
	if got := avoid.Scalars["metadata.cluster.key"].Value; got != "message|sha256:shared-clock-failure" {
		t.Fatalf("metadata.cluster.key = %q, want shared message fingerprint key", got)
	}
}

func TestDistillRejectsBlankExplicitInput(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", " ", "--format", "json"}, false)

	if code != ExitUsage {
		t.Fatalf("distill blank input exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "distill --input must be a non-empty path") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestDistillReviewRequiredFalseStillDraftsCandidateRules(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "review_required: true", "review_required: false")
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-review-gate-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0701","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":701,"commit":"abc701","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_review_gate","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_review_gate","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/701"]}`,
		`{"experience_id":"exp_0702","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-08T10:00:00Z","pr":702,"commit":"abc702","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_review_gate","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_review_gate","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/702"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	avoid := loadRuleDocsByKindForTest(t, tempDir)["avoid"]
	if avoid.Scalars["status"].Value != "candidate" || avoid.Scalars["review.label"].Value == "accepted" {
		t.Fatalf("review_required=false auto-accepted draft: status %q review %q", avoid.Scalars["status"].Value, avoid.Scalars["review.label"].Value)
	}
	if avoid.Scalars["metadata.review_required"].Value != "false" {
		t.Fatalf("review_required metadata = %#v", avoid.Scalars["metadata.review_required"])
	}
}

func TestCheckDisclosesOpenAICompatibleProviderBoundary(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	enableProviderForTest(t, tempDir, "openai_compatible", "gpt-test", "https://openai-compatible.example.test/v1", "RELIA_OPENAI_COMPATIBLE_API_KEY", "5.00")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("check exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Warnings) == 0 {
		t.Fatal("expected provider disclosure warning")
	}
	var disclosure Finding
	for _, warning := range result.Warnings {
		if warning.Type == "provider_data_disclosure" {
			disclosure = warning
			break
		}
	}
	if disclosure.Type == "" || !strings.Contains(disclosure.Message, "redacted experience records") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestCheckRejectsProviderBaseURLUserInfo(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	enableProviderForTest(t, tempDir, "openai_compatible", "gpt-test", "https://token@openai-compatible.example.test/v1", "RELIA_OPENAI_COMPATIBLE_API_KEY", "5.00")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitUsage {
		t.Fatalf("check user-info URL exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "must not include user info") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if strings.Contains(stdout, "token@") || strings.Contains(stderr, "token@") {
		t.Fatalf("provider URL user info leaked in output: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestDistillProviderPlanReportsCostAndFailsClosedWithoutGrant(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	enableProviderForTest(t, tempDir, "anthropic", "claude-test", "https://api.anthropic.example.test", "RELIA_ANTHROPIC_API_KEY", "5.00")
	inputRel := filepath.ToSlash(filepath.Join("fixtures", "provider-distill.jsonl"))
	writeFileForTest(t, filepath.Join(tempDir, filepath.FromSlash(inputRel)), `{"experience_id":"exp_0901","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":901,"commit":"abc901","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_provider_plan","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_provider","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/901"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)

	if code != ExitDependency {
		t.Fatalf("distill provider exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "model_provider_endpoint") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	plan := result.Data["provider_plan"].(map[string]any)
	if plan["provider"] != "anthropic" || plan["adapter"] != "anthropic_messages_http" || plan["model"] != "claude-test" {
		t.Fatalf("provider_plan = %#v", plan)
	}
	cost := plan["cost"].(map[string]any)
	if cost["estimated_cost_usd"].(float64) <= 0 ||
		cost["input_tokens_estimated"].(float64) <= 0 ||
		cost["output_tokens_estimated"].(float64) <= 0 {
		t.Fatalf("cost estimate = %#v", cost)
	}
	if plan["provider_call_attempted"] != false || plan["approval_required"] != "model_provider_endpoint" {
		t.Fatalf("provider execution boundary = %#v", plan)
	}
	if matches, err := filepath.Glob(filepath.Join(tempDir, "memory", "rules", "*.yaml")); err != nil {
		t.Fatal(err)
	} else if len(matches) != 0 {
		t.Fatalf("provider-gated distill wrote memory rules without grant: %#v", matches)
	}
}

func TestDistillProviderPlanRespectsConfiguredCostCap(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	enableProviderForTest(t, tempDir, "anthropic", "claude-test", "https://api.anthropic.example.test", "RELIA_ANTHROPIC_API_KEY", "0.000001")
	inputRel := filepath.ToSlash(filepath.Join("fixtures", "provider-cost-cap.jsonl"))
	writeFileForTest(t, filepath.Join(tempDir, filepath.FromSlash(inputRel)), `{"experience_id":"exp_0911","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":911,"commit":"abc911","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_provider_cap","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_provider_cap","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/911"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)

	if code != ExitDependency {
		t.Fatalf("distill provider cap exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "max_cost_usd_per_run") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	cost := result.Data["provider_plan"].(map[string]any)["cost"].(map[string]any)
	if cost["cap_status"] != "exceeded" {
		t.Fatalf("cost estimate = %#v", cost)
	}
}

func TestReviewApproveEditRejectTransitions(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	rulePath := filepath.Join(tempDir, "memory", "rules", "billing-time.yaml")
	writeFileForTest(t, rulePath, `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-time
kind: avoid
status: candidate
statement: Avoid direct billing clock calls.
scope:
  paths:
    - packages/billing/invoice.py
confidence: 0.6
evidence:
  count: 2
  contradictions: 0
  experiences:
    - exp_0601
    - exp_0602
provenance:
  - pr: 601
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/601
review:
  label: suggested
  statement_origin: cluster_summary
metadata:
  confidence_label: medium
  lifecycle_reason: human review required before activation
  confidence_inputs:
    evidence_count: 2
    recency_weight: 1
    contradictions: 0
    flake_discount: 0
    extraction_confidence: 1
    drafting_model_weight: 0
  decay:
    half_life_days: 90
    latest_evidence_at: 2026-04-08T10:00:00Z
    oldest_evidence_at: 2026-04-01T10:00:00Z
    anchor_recorded_at: 2026-04-08T10:00:00Z
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "review", "edit", "--rule", "billing-time", "--statement", "Use the billing clock fixture instead of direct UTC calls."}, false)
	if code != ExitSuccess {
		t.Fatalf("review edit exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	edited := parseRuleDocForTest(t, readRuleByIDForTest(t, tempDir, "billing-time"))
	if edited.Scalars["status"].Value != "candidate" ||
		edited.Scalars["review.label"].Value != "suggested" ||
		edited.Scalars["review.statement_origin"].Value != "human_authored" ||
		edited.Scalars["statement"].Value != "Use the billing clock fixture instead of direct UTC calls." {
		t.Fatalf("edited rule = %#v", edited.Scalars)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "review", "approve", "--rule", "billing-time"}, false)
	if code != ExitSuccess {
		t.Fatalf("review approve exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	approved := parseRuleDocForTest(t, readRuleByIDForTest(t, tempDir, "billing-time"))
	if approved.Scalars["status"].Value != "active" || approved.Scalars["review.label"].Value != "accepted" {
		t.Fatalf("approved rule = %#v", approved.Scalars)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "review", "reject", "--rule", "billing-time", "--reason", "superseded by a narrower billing rule"}, false)
	if code != ExitSuccess {
		t.Fatalf("review reject exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	rejected := parseRuleDocForTest(t, readRuleByIDForTest(t, tempDir, "billing-time"))
	if rejected.Scalars["status"].Value != "retired" ||
		rejected.Scalars["review.label"].Value != "needs_user_input" ||
		!strings.Contains(rejected.Scalars["metadata.lifecycle_reason"].Value, "superseded") {
		t.Fatalf("rejected rule = %#v", rejected.Scalars)
	}
}

func TestDistillMarksContradictedAndStaleRules(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-lifecycle-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0201","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-03-01T10:00:00Z","pr":201,"commit":"abc201","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_rounding","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/201"]}`,
		`{"experience_id":"exp_0202","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-03-08T10:00:00Z","pr":202,"commit":"abc202","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"merged_clean","terminal_state":"passed","signature_id":"sig_billing_rounding","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/202"]}`,
		`{"experience_id":"exp_0301","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":301,"commit":"abc301","paths":["packages/removed/legacy.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_removed_legacy","signature_class":"test_failure","check_name":"pytest-legacy","signature_key":"tests/legacy/test_removed.py::test_legacy","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/301"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	rules := loadRuleDocsByStatusForTest(t, tempDir)
	contradicted := rules["contradicted"]
	stale := rules["stale"]
	if contradicted.Scalars["review.label"].Value != "needs_user_input" ||
		contradicted.Scalars["evidence.contradictions"].Value != "1" {
		t.Fatalf("contradicted rule = %#v", contradicted.Scalars)
	}
	if stale.Scalars["metadata.lifecycle_reason"].Value != "all scoped paths are missing from the working tree" {
		t.Fatalf("stale rule metadata = %#v", stale.Scalars)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "review", "--rule", stale.Scalars["id"].Value, "--label", "accepted"}, false)
	if code != ExitValidation {
		t.Fatalf("review stale exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" || !strings.Contains(result.Errors[0].Message, "stale") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestReviewFailsClosedWithoutMutatingInvalidRule(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	rulePath := filepath.Join(tempDir, "memory", "rules", "invalid-candidate.yaml")
	writeFileForTest(t, rulePath, `object_type: relia.memory_rule
schema_version: "1.0"
id: invalid-candidate
kind: avoid
status: candidate
statement: Avoid direct billing clock calls.
scope:
  paths:
    - packages/billing/invoice.py
confidence: 0.72
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0501
review:
  label: suggested
  statement_origin: cluster_summary
metadata:
  confidence_label: medium
`)
	before, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "review", "--rule", "invalid-candidate", "--label", "accepted"}, false)

	if code != ExitValidation {
		t.Fatalf("review exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" || !strings.Contains(result.Errors[0].Message, "provenance") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	after, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("review mutated invalid rule despite failed validation:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestReviewRejectsEditOnlyFlagsWithoutEditAction(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)

	stdout, stderr, code := runForTest(t, []string{"--json", "review", "--rule", "billing-time", "--statement", "Use a safer billing clock."}, false)

	if code != ExitUsage {
		t.Fatalf("review edit-only flag exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" || !strings.Contains(result.Errors[0].Message, "require review edit") {
		t.Fatalf("errors = %#v", result.Errors)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "review", "--rule", "billing-time", "--reason", "not enough evidence"}, false)
	if code != ExitUsage {
		t.Fatalf("review reason-only flag exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result = decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" || !strings.Contains(result.Errors[0].Message, "requires review reject") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestReviewEditRejectsMissingScopePathWithoutMutatingRule(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	rulePath := filepath.Join(tempDir, "memory", "rules", "billing-time.yaml")
	writeFileForTest(t, rulePath, `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-time
kind: avoid
status: candidate
statement: Avoid direct billing clock calls.
scope:
  paths:
    - packages/billing/invoice.py
confidence: 0.6
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0601
provenance:
  - pr: 601
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/601
review:
  label: suggested
  statement_origin: cluster_summary
metadata:
  confidence_label: medium
  confidence_inputs:
    evidence_count: 1
    recency_weight: 1
    contradictions: 0
    flake_discount: 0
    extraction_confidence: 1
    drafting_model_weight: 0
  decay:
    half_life_days: 90
    latest_evidence_at: 2026-04-08T10:00:00Z
    oldest_evidence_at: 2026-04-01T10:00:00Z
    anchor_recorded_at: 2026-04-08T10:00:00Z
`)
	before, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "review", "edit", "--rule", "billing-time", "--scope-path", "packages/billing/missing.py"}, false)

	if code != ExitValidation {
		t.Fatalf("review edit missing scope exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" || !strings.Contains(result.Errors[0].Message, "scope path does not exist") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	after, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("review edit mutated rule despite invalid scope:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestDistillMarksAnyLaterContradictionAsContradicted(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-later-contradiction.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0401","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-03-01T10:00:00Z","pr":401,"commit":"abc401","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_tax_rounding","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_tax_rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/401"]}`,
		`{"experience_id":"exp_0402","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-03-08T10:00:00Z","pr":402,"commit":"abc402","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_tax_rounding","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_tax_rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/402"]}`,
		`{"experience_id":"exp_0403","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-03-15T10:00:00Z","pr":403,"commit":"abc403","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"merged_clean","terminal_state":"passed","signature_id":"sig_billing_tax_rounding","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_tax_rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/403"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	rules := loadRuleDocsByStatusForTest(t, tempDir)
	contradicted := rules["contradicted"]
	if contradicted.Scalars["evidence.contradictions"].Value != "1" ||
		contradicted.Scalars["review.label"].Value != "needs_user_input" {
		t.Fatalf("contradicted rule = %#v", contradicted.Scalars)
	}
}

func TestDistillAvoidContradictionsIgnoreOlderPositiveEvidence(t *testing.T) {
	timestamp := func(value string) time.Time {
		t.Helper()
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	failures := []backtestExperience{
		{RecordedAt: timestamp("2026-04-10T10:00:00Z")},
	}
	positives := []backtestExperience{
		{RecordedAt: timestamp("2026-04-01T10:00:00Z")},
	}
	if got := distillAvoidContradictions(failures, positives); got != 0 {
		t.Fatalf("older positive evidence counted as contradiction: %d", got)
	}
	positives = append(positives, backtestExperience{RecordedAt: timestamp("2026-04-12T10:00:00Z")})
	if got := distillAvoidContradictions(failures, positives); got != 1 {
		t.Fatalf("later positive evidence contradictions = %d, want 1", got)
	}
}

func TestYAMLScalarForWriteQuotesColonSpace(t *testing.T) {
	quoted := yamlScalarForWrite("build: lint")
	if quoted != `"build: lint"` {
		t.Fatalf("yamlScalarForWrite did not quote colon-space scalar: %q", quoted)
	}
	document := parseRuleDocForTest(t, "statement: "+quoted+"\n")
	if got := document.Scalars["statement"].Value; got != "build: lint" {
		t.Fatalf("parsed statement = %q", got)
	}
}

func TestCheckRejectsZeroMatchAttributionConfigWithConcreteRef(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	configPath := filepath.Join(tempDir, "relia.yaml")
	replaceInFile(t, configPath, `  coauthor_trailers:
    - Claude
    - Claude Code`, `  coauthor_trailers: []`)
	replaceInFile(t, configPath, `  pr_labels:
    - agent-authored`, `  pr_labels: []`)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "zero agent matchers") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
	if !strings.Contains(result.Errors[0].Ref, "relia.yaml:") {
		t.Fatalf("error ref = %q, want concrete relia.yaml line", result.Errors[0].Ref)
	}
}

func TestCheckRejectsNaNRecurrenceGateThreshold(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	configPath := filepath.Join(tempDir, "relia.yaml")
	replaceInFile(t, configPath, `gate:
  enabled: false`, `gate:
  enabled: true
  max_error_recurrence_rate: NaN`)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "gate.max_error_recurrence_rate must be a number between 0 and 1") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
	if !strings.Contains(result.Errors[0].Ref, "relia.yaml:") {
		t.Fatalf("error ref = %q, want concrete relia.yaml line", result.Errors[0].Ref)
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
	for _, token := range []string{"version: 1", "schema_version: \"1.0\"", "local_only: true", "fail_closed: true", "embeddings: signature", "advisory_only: true"} {
		if !bytes.Contains(content, []byte(token)) {
			t.Fatalf("relia.yaml missing %q:\n%s", token, content)
		}
	}
	for _, dir := range artifactSkeletonDirs {
		if info, err := os.Stat(filepath.Join(tempDir, dir)); err != nil || !info.IsDir() {
			t.Fatalf("expected artifact skeleton dir %s: info=%#v err=%v", dir, info, err)
		}
	}
	ignoreContent, err := os.ReadFile(filepath.Join(tempDir, ".gitignore"))
	if err != nil {
		t.Fatalf("expected .gitignore to be created: %v", err)
	}
	if !bytes.Contains(ignoreContent, []byte(".relia/")) {
		t.Fatalf(".gitignore missing .relia/:\n%s", ignoreContent)
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
		Examples      []CommandResult `json:"examples"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode examples: %v", err)
	}
	if payload.ObjectType != "relia.command_result_examples" {
		t.Fatalf("object_type = %q", payload.ObjectType)
	}
	codes := make([]int, 0, len(payload.Examples))
	for _, example := range payload.Examples {
		if example.ObjectType != "relia.command_result" {
			t.Fatalf("example object_type = %q", example.ObjectType)
		}
		if example.SchemaVersion != "1.0" {
			t.Fatalf("example schema_version = %q", example.SchemaVersion)
		}
		if example.Metadata["schema_version"] != "1.0" {
			t.Fatalf("example metadata = %#v", example.Metadata)
		}
		if example.ExitCode < ExitSuccess || example.ExitCode > ExitProvenanceIntegrity {
			t.Fatalf("unexpected exit code in example: %d", example.ExitCode)
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

func TestPhase0SchemasDeclareMetadata(t *testing.T) {
	root := findRepoRootForTest(t)
	if commandErr := validateSchemaContracts(root); commandErr != nil {
		t.Fatalf("schema contracts failed: %#v", commandErr)
	}
}

func TestRecurrenceReportSchemaKeepsT8FieldsOptionalForV1(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(findRepoRootForTest(t), "schemas", "recurrence-report.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(content, &schema); err != nil {
		t.Fatal(err)
	}
	requiredValues, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema required field = %#v", schema["required"])
	}
	required := map[string]bool{}
	for _, value := range requiredValues {
		required[fmt.Sprint(value)] = true
	}
	for _, field := range []string{"metrics", "top_repeated_mistakes", "diagnostics", "operator_feedback", "badge"} {
		if required[field] {
			t.Fatalf("%s must stay optional while recurrence-report schema_version remains 1.0", field)
		}
	}
}

func TestCheckReportsPhase0ContractRefs(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got, ok := result.Data["schema_contracts"].(float64); !ok || int(got) != len(requiredSchemaFiles) {
		t.Fatalf("schema_contracts = %#v, want %d", result.Data["schema_contracts"], len(requiredSchemaFiles))
	}
	if result.Data["privacy_default"] != "local_only" {
		t.Fatalf("privacy_default = %#v", result.Data["privacy_default"])
	}
	if len(result.Artifacts) <= len(requiredSchemaFiles) {
		t.Fatalf("expected schema artifacts in result: %#v", result.Artifacts)
	}
}

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

func TestDemoAssessIgnoresUnrelatedRuleWithoutCitationURLs(t *testing.T) {
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
review:
  label: accepted
  statement_origin: human_authored
metadata: {}
`)
	writeFileForTest(t, filepath.Join(tempDir, "unknown.diff"), `diff --git a/packages/search/query.py b/packages/search/query.py
--- a/packages/search/query.py
+++ b/packages/search/query.py
@@ -1,2 +1,3 @@
 def normalize_query(value):
-    return value.strip().lower()
+    normalized = value.strip().lower()
+    return " ".join(normalized.split())
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "unknown.diff"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	assessment := decodeAssessmentFromResult(t, decodeResult(t, stdout))
	if assessment.RiskLevel != "no_coverage" {
		t.Fatalf("risk_level = %q, want no_coverage", assessment.RiskLevel)
	}
	if len(assessment.Matches) != 0 || len(assessment.Citations) != 0 {
		t.Fatalf("assessment served unrelated uncited rule: matches=%#v citations=%#v", assessment.Matches, assessment.Citations)
	}
}

func TestDemoAssessIgnoresInactiveStaleRuleWithRemovedPath(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "stale-billing.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: stale-billing-fixture
kind: avoid
status: stale
statement: Historical billing rule for a removed module.
scope:
  paths:
    - packages/removed-billing/
confidence: 0.86
evidence:
  count: 1
  contradictions: 1
  experiences:
    - exp_0142
provenance:
  - pr: 142
    outcome: ci_failure
review:
  label: suggested
  statement_origin: human_authored
metadata: {}
`)
	writeFileForTest(t, filepath.Join(tempDir, "unknown.diff"), `diff --git a/packages/search/query.py b/packages/search/query.py
--- a/packages/search/query.py
+++ b/packages/search/query.py
@@ -1,2 +1,3 @@
 def normalize_query(value):
-    return value.strip().lower()
+    normalized = value.strip().lower()
+    return " ".join(normalized.split())
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "unknown.diff"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	assessment := decodeAssessmentFromResult(t, decodeResult(t, stdout))
	if assessment.RiskLevel != "no_coverage" {
		t.Fatalf("risk_level = %q, want no_coverage", assessment.RiskLevel)
	}
	if len(assessment.Matches) != 0 || len(assessment.Citations) != 0 {
		t.Fatalf("assessment served inactive stale rule: matches=%#v citations=%#v", assessment.Matches, assessment.Citations)
	}
}

func TestDemoAssessRejectsActiveRuleWithUnknownScopePath(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "unknown-billing.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: unknown-billing-fixture
kind: avoid
status: active
statement: Do not depend on the misspelled billing package.
scope:
  paths:
    - pakcages/billing/
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
	writeFileForTest(t, filepath.Join(tempDir, "unknown.diff"), `diff --git a/pakcages/billing/invoice.py b/pakcages/billing/invoice.py
--- a/pakcages/billing/invoice.py
+++ b/pakcages/billing/invoice.py
@@ -1,2 +1,3 @@
 def rollover_day():
-    return "2026-01-01"
+    return datetime.utcnow().strftime("%Y-%m-%d")
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "unknown.diff"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "scope path does not exist") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsActiveRuleWithoutStatement(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "billing-time.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-time-fixture
kind: avoid
status: active
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

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "missing required key statement") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsActiveNonMemoryRuleArtifact(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "not-a-memory-rule.yaml"), `object_type: relia.coverage_map
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

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "object_type must be relia.memory_rule") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsActiveUnacceptedMemoryRule(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "unaccepted-billing.yaml"), `object_type: relia.memory_rule
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
  label: suggested
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

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "review.label must be accepted") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsActiveRuleWithoutStatementOrigin(t *testing.T) {
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

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "review.statement_origin") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsActiveRuleWithoutEvidenceExperiences(t *testing.T) {
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
  experiences: []
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

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "at least one experience") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsActiveRuleWithZeroEvidenceCount(t *testing.T) {
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
  count: 0
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

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "evidence.count must be at least 1") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsActiveRuleWithoutEvidenceContradictions(t *testing.T) {
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

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "evidence.contradictions") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsActiveRuleWithMissingProvenanceOutcome(t *testing.T) {
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

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "provenance entry missing outcome") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsActiveRuleWithMissingProvenancePR(t *testing.T) {
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
  - outcome: review_correction
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

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "provenance entry missing pr") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsActiveRuleWithScalarProvenanceEntry(t *testing.T) {
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
  - malformed
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

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "provenance entries must include pr and outcome") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessIgnoresHunkBodyDiffMarkersAsPaths(t *testing.T) {
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
	writeFileForTest(t, filepath.Join(tempDir, "unknown.diff"), `diff --git a/packages/search/query.py b/packages/search/query.py
--- a/packages/search/query.py
+++ b/packages/search/query.py
@@ -1,2 +1,4 @@
 def normalize_query(value):
-    return value.strip().lower()
++ packages/billing/invoice.py
+    normalized = value.strip().lower()
+    return " ".join(normalized.split())
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "unknown.diff"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	assessment := decodeAssessmentFromResult(t, decodeResult(t, stdout))
	if assessment.RiskLevel != "no_coverage" {
		t.Fatalf("risk_level = %q, want no_coverage", assessment.RiskLevel)
	}
	if len(assessment.Matches) != 0 || len(assessment.Citations) != 0 {
		t.Fatalf("assessment used hunk body marker as path: matches=%#v citations=%#v", assessment.Matches, assessment.Citations)
	}
}

func TestDemoAssessMatchesPureRenameSourcePath(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "billing-time.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-time-fixture
kind: avoid
status: active
statement: Keep billing invoice changes inside the governed workflow.
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
	writeFileForTest(t, filepath.Join(tempDir, "rename.diff"), `diff --git a/packages/billing/invoice.py b/packages/archive/invoice.py
similarity index 100%
rename from packages/billing/invoice.py
rename to packages/archive/invoice.py
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", "rename.diff"}, false)

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

func TestDemoAssessRejectsMatchedRuleWithInvalidCitationURL(t *testing.T) {
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
    url: not-a-url
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

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "provenance_integrity_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "citation URL") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsMatchedRuleWithNonPullCitationURL(t *testing.T) {
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
    url: https://github.com/acme/billing-service/actions/runs/981
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

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "/pull/<number>") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsMatchedRuleWithMismatchedCitationPR(t *testing.T) {
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
    url: https://github.com/acme/billing-service/pull/999
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

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "pull number must match provenance pr") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsMatchedRuleWithOutOfRangeConfidence(t *testing.T) {
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
confidence: 1.2
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

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "provenance_integrity_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "confidence") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsMatchedRuleWithEmptyID(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "memory", "rules", "billing-time.yaml"), `object_type: relia.memory_rule
schema_version: "1.0"
id: ""
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

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "provenance_integrity_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "id") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestDemoAssessRejectsMatchedRuleWithoutCitationURLs(t *testing.T) {
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

	if code != ExitProvenanceIntegrity {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "provenance_integrity_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckRejectsUnsafePrivacyConfig(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "send_code: false", "send_code: true")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckFailsClosedForDisabledRedaction(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "fail_closed: true", "fail_closed: false")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "redaction_safety_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckRequiresLocalModelManifest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckRejectsIncompleteLocalModelManifest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), `{
  "model_id": "text-embedding-test"
}
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "missing required field") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestModelsPullRecordsLocalManifestWithoutNetwork(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	artifactContent := []byte("deterministic local model artifact")
	artifactRel := filepath.ToSlash(filepath.Join(".relia", "models", "artifact.bin"))
	writeFileForTest(t, filepath.Join(tempDir, filepath.FromSlash(artifactRel)), string(artifactContent))
	digest := sha256.Sum256(artifactContent)

	stdout, stderr, code := runForTest(t, []string{
		"--json",
		"models",
		"pull",
		"--model-id", "text-embedding-test",
		"--version", "2026-06-22",
		"--source-url", "https://example.test/model.bin",
		"--license", "Apache-2.0",
		"--digest", fmt.Sprintf("%x", digest),
		"--cache-path", artifactRel,
		"--update-policy", "manual",
		"--rollback-policy", "delete artifact and restore signature embeddings",
	}, false)

	if code != ExitSuccess {
		t.Fatalf("models pull exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "models pull" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	manifestPath := filepath.Join(tempDir, ".relia", "models", "manifest.json")
	var manifest map[string]any
	readJSONFileForTest(t, manifestPath, &manifest)
	if manifest["model_id"] != "text-embedding-test" ||
		manifest["version"] != "2026-06-22" ||
		manifest["source_url"] != "https://example.test/model.bin" ||
		manifest["license"] != "Apache-2.0" ||
		manifest["cache_path"] != artifactRel ||
		manifest["update_policy"] != "manual" ||
		manifest["rollback_policy"] == "" ||
		manifest["status"] != "ready" {
		t.Fatalf("manifest = %#v", manifest)
	}
	if commandErr := validateLocalModelManifest(tempDir, yamlScalar{Value: ".relia/models/manifest.json", Line: 1}); commandErr != nil {
		t.Fatalf("manifest did not validate after models pull: %#v", commandErr)
	}
}

func TestModelsPullRejectsCachePathAtManifestPath(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	digest := sha256.Sum256([]byte("manifest collision payload"))

	stdout, stderr, code := runForTest(t, []string{
		"--json",
		"models",
		"pull",
		"--model-id", "text-embedding-test",
		"--version", "2026-06-22",
		"--source-url", "https://example.test/model.bin",
		"--license", "Apache-2.0",
		"--digest", fmt.Sprintf("%x", digest),
		"--cache-path", ".relia/models/manifest.json",
		"--update-policy", "manual",
		"--rollback-policy", "delete artifact and restore signature embeddings",
	}, false)

	if code != ExitUsage {
		t.Fatalf("models pull manifest cache exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" || !strings.Contains(result.Errors[0].Message, "must not equal") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "models", "manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifest path exists after rejected models pull: %v", err)
	}
}

func TestCheckValidatesLocalModelManifestDigest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	artifactContent := []byte("deterministic local model artifact")
	artifactRel := filepath.Join(".relia", "models", "artifact.bin")
	writeFileForTest(t, filepath.Join(tempDir, artifactRel), string(artifactContent))
	digest := sha256.Sum256(artifactContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), fmt.Sprintf(`{
  "model_id": "text-embedding-test",
  "version": "2026-06-22",
  "source_url": "https://example.test/model.bin",
  "license": "Apache-2.0",
  "digest": "%x",
  "cache_path": "%s",
  "update_policy": "manual",
  "rollback_policy": "delete artifact and restore signature embeddings"
}
`, digest, filepath.ToSlash(artifactRel)))

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestCheckRejectsStaleLocalModelManifest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	artifactContent := []byte("deterministic local model artifact")
	artifactRel := filepath.Join(".relia", "models", "artifact.bin")
	writeFileForTest(t, filepath.Join(tempDir, artifactRel), string(artifactContent))
	digest := sha256.Sum256(artifactContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), fmt.Sprintf(`{
  "model_id": "text-embedding-test",
  "version": "2026-06-22",
  "source_url": "https://example.test/model.bin",
  "license": "Apache-2.0",
  "digest": "%x",
  "cache_path": "%s",
  "update_policy": "manual",
  "rollback_policy": "delete artifact and restore signature embeddings",
  "status": "stale"
}
`, digest, filepath.ToSlash(artifactRel)))

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" || !strings.Contains(result.Errors[0].Message, "stale") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestCheckRejectsEscapedLocalModelCachePath(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	artifactContent := []byte("outside model artifact")
	outsideRel := "outside-model.bin"
	writeFileForTest(t, filepath.Join(tempDir, outsideRel), string(artifactContent))
	digest := sha256.Sum256(artifactContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), fmt.Sprintf(`{
  "model_id": "text-embedding-test",
  "version": "2026-06-22",
  "source_url": "https://example.test/model.bin",
  "license": "Apache-2.0",
  "digest": "%x",
  "cache_path": "../%s",
  "update_policy": "manual",
  "rollback_policy": "delete artifact and restore signature embeddings"
}
`, digest, outsideRel))

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "inside the repository") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

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

func decodeBacktestReportFromResult(t *testing.T, result CommandResult) recurrenceReport {
	t.Helper()
	encoded, err := json.Marshal(result.Data["report"])
	if err != nil {
		t.Fatalf("encode nested backtest report: %v", err)
	}
	var report recurrenceReport
	if err := json.Unmarshal(encoded, &report); err != nil {
		t.Fatalf("decode nested backtest report from %s: %v", encoded, err)
	}
	return report
}

func decodeJSONLines(t *testing.T, content string) []map[string]any {
	t.Helper()

	var records []map[string]any
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode JSON line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func loadRuleDocsByKindForTest(t *testing.T, root string) map[string]yamlDocument {
	t.Helper()
	return loadRuleDocsByScalarForTest(t, root, "kind")
}

func loadRuleDocsByStatusForTest(t *testing.T, root string) map[string]yamlDocument {
	t.Helper()
	return loadRuleDocsByScalarForTest(t, root, "status")
}

func findRuleDocByEvidenceForTest(t *testing.T, root string, kind string, experienceIDs []string) yamlDocument {
	t.Helper()
	for _, document := range loadRuleDocsForTest(t, root) {
		if document.Scalars["kind"].Value != kind {
			continue
		}
		if stringSlicesEqual(yamlScalarValuesForTest(document.Lists["evidence.experiences"]), experienceIDs) {
			return document
		}
	}
	t.Fatalf("could not find %s rule with evidence experiences %#v", kind, experienceIDs)
	return yamlDocument{}
}

func distillClusterKeyForTest(kind string, signatureID string, signatureClass string, checkName string, signatureKey string) string {
	return distillClusterKey(experienceRecord{
		Outcome: experienceOutcome{
			Kind: kind,
			Signature: experienceSignature{
				SignatureID: signatureID,
			},
		},
		Metadata: map[string]any{
			"signature": map[string]any{
				"class":      signatureClass,
				"check_name": checkName,
				"key":        signatureKey,
			},
		},
	})
}

func yamlScalarValuesForTest(scalars []yamlScalar) []string {
	values := make([]string, 0, len(scalars))
	for _, scalar := range scalars {
		values = append(values, scalar.Value)
	}
	return values
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assertReportDiagnosticTypes(t *testing.T, diagnostics []reportDiagnostic, wantTypes []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, diagnostic := range diagnostics {
		seen[diagnostic.Type] = true
		if diagnostic.Status == "" || diagnostic.Message == "" || diagnostic.Ref == "" {
			t.Fatalf("diagnostic missing operator-visible details: %#v", diagnostic)
		}
	}
	for _, want := range wantTypes {
		if !seen[want] {
			t.Fatalf("diagnostics missing %q: %#v", want, diagnostics)
		}
	}
}

func loadRuleDocsByScalarForTest(t *testing.T, root string, scalar string) map[string]yamlDocument {
	t.Helper()
	docs := map[string]yamlDocument{}
	for _, document := range loadRuleDocsForTest(t, root) {
		key := document.Scalars[scalar].Value
		if key == "" {
			t.Fatalf("rule missing scalar %s: %#v", scalar, document.Scalars)
		}
		docs[key] = document
	}
	return docs
}

func loadRuleDocsForTest(t *testing.T, root string) []yamlDocument {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "memory", "rules", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("expected generated memory rule YAML files")
	}
	var docs []yamlDocument
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		document := parseRuleDocForTest(t, string(content))
		docs = append(docs, document)
	}
	return docs
}

func readRuleByIDForTest(t *testing.T, root string, ruleID string) string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "memory", "rules", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		document := parseRuleDocForTest(t, string(content))
		if document.Scalars["id"].Value == ruleID {
			return string(content)
		}
	}
	t.Fatalf("could not find generated rule %q", ruleID)
	return ""
}

func parseRuleDocForTest(t *testing.T, content string) yamlDocument {
	t.Helper()
	document, err := parseYAMLDocument(content)
	if err != nil {
		t.Fatalf("parse rule YAML:\n%s\n%v", content, err)
	}
	return document
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

func setupContractRepo(t *testing.T) string {
	t.Helper()

	sourceRoot := findRepoRootForTest(t)
	tempDir := t.TempDir()
	files := map[string]string{
		"AGENTS.md":              "repo contract\n",
		"WORKFLOW.md":            "workflow contract\n",
		"README.md":              "readme\n",
		"Makefile":               "prepush-full:\n",
		".tool-versions":         "golang 1.26.4\n",
		"go.mod":                 "module github.com/Clyra-AI/relia\n\ngo 1.26.4\n",
		"relia.yaml":             defaultConfigYAML(),
		"docs/product/prd.md":    "prd\n",
		"docs/dev/dev_guides.md": "dev guides\n",
		"docs/architecture/architecture_guides.md": "architecture guides\n",
		"packages/billing/.keep":                   "\n",
		"tests/.keep":                              "\n",
		".github/required-checks.json":             "{}\n",
		".github/workflows/validate.yml":           "name: validate\n",
		".github/workflows/codeql.yml":             "name: codeql\n",
		".factory/factoryd.example.json":           "{}\n",
		".factory/factoryd.autoship.example.json":  "{}\n",
	}
	for rel, content := range files {
		writeFileForTest(t, filepath.Join(tempDir, rel), content)
	}
	for _, rel := range requiredSchemaFiles {
		content, err := os.ReadFile(filepath.Join(sourceRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		writeFileForTest(t, filepath.Join(tempDir, rel), string(content))
	}
	return tempDir
}

func enableProviderForTest(t *testing.T, root string, provider string, model string, baseURL string, credentialEnv string, maxCost string) {
	t.Helper()
	replaceInFile(t, filepath.Join(root, "relia.yaml"), `distill:
  embeddings: signature
  provider: none
  model: ""
  base_url: ""
  credential_env: ""
  max_cost_usd_per_run: 0
  input_cost_usd_per_1k_tokens: 0
  output_cost_usd_per_1k_tokens: 0
  review_required: true`, fmt.Sprintf(`distill:
  embeddings: signature
  provider: %s
  model: %s
  base_url: %s
  credential_env: %s
  max_cost_usd_per_run: %s
  input_cost_usd_per_1k_tokens: 0.001
  output_cost_usd_per_1k_tokens: 0.002
  review_required: true`, provider, model, baseURL, credentialEnv, maxCost))
}

func writeFileForTest(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func replaceInFile(t *testing.T, path string, old string, new string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	next := strings.Replace(string(content), old, new, 1)
	if next == string(content) {
		t.Fatalf("expected to replace %q in %s", old, path)
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		t.Fatal(err)
	}
}
