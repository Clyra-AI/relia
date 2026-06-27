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
		{"--json", "backtest"},
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

func TestParseUnifiedDiffTouchedPathsKeepsQuotedSpaces(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git "a/docs/api guide.md" "b/docs/api guide.md"
--- "a/docs/api guide.md"
+++ "b/docs/api guide.md"
@@ -1 +1 @@
-old
+new
`), "quoted.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"docs/api guide.md"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsKeepsUnquotedSpaces(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/docs/api guide.md b/docs/api guide.md
--- a/docs/api guide.md
+++ b/docs/api guide.md
@@ -1 +1 @@
-old
+new
`), "unquoted-spaces.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"docs/api guide.md"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsSupportsPlainUnifiedDiff(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`--- packages/billing/invoice.py
+++ packages/billing/invoice.py
@@ -1 +1 @@
-old
+new
`), "plain.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"packages/billing/invoice.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsPreservesPlainDiffLiteralAPrefix(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`--- a/foo.py
+++ a/foo.py
@@ -1 +1 @@
-old
+new
`), "plain-a-prefix.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"a/foo.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsPreservesNoPrefixGitLiteralAPrefix(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/foo.py a/foo.py
--- a/foo.py
+++ a/foo.py
@@ -1 +1 @@
-old
+new
`), "no-prefix-git-a-prefix.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"a/foo.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsIgnoresPlainHeaderLookalikesInsideHunks(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/packages/search/query.py b/packages/search/query.py
--- a/packages/search/query.py
+++ b/packages/search/query.py
@@ -1,4 +1,5 @@
 def normalize_query(value):
--- packages/billing/invoice.py
+++ packages/billing/invoice.py
@@ -8,2 +9,3 @@
     return value.strip().lower()
`), "hunk-lookalike.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"packages/search/query.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsSkipsAmbiguousGitHeaderSeparator(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/foo b/bar.txt b/foo b/bar.txt
--- a/foo b/bar.txt
+++ b/foo b/bar.txt
@@ -1 +1 @@
-old
+new
`), "ambiguous-git-header.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"foo b/bar.txt"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsRecognizesSubsequentPlainFileHeaders(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`--- packages/search/query.py
+++ packages/search/query.py
@@ -1 +1 @@
-old
+new
--- packages/billing/invoice.py
+++ packages/billing/invoice.py
@@ -1 +1 @@
-old
+new
`), "multi-file-plain.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"packages/billing/invoice.py", "packages/search/query.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsPreservesRenameMetadataLiteralAPrefix(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/a/foo.py b/a/bar.py
similarity index 100%
rename from a/foo.py
rename to a/bar.py
`), "rename-literal-a.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"a/bar.py", "a/foo.py"}) {
		t.Fatalf("paths = %#v", paths)
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
