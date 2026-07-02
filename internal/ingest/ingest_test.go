package ingest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEventsAcceptsObjectArrayEnvelopeAndJSONL(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "single object",
			input: `{"repo":"acme/billing","pr":142}`,
			want:  1,
		},
		{
			name:  "array",
			input: `[{"pr":142},{"pr":143}]`,
			want:  2,
		},
		{
			name:  "events envelope",
			input: `{"events":[{"pr":142},{"pr":143}]}`,
			want:  2,
		},
		{
			name:  "jsonl",
			input: "{\"pr\":142}\n{\"pr\":143}\n",
			want:  2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseEvents([]byte(tc.input), "input.json")
			if err != nil {
				t.Fatalf("ParseEvents error: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("events = %d, want %d", len(got), tc.want)
			}
		})
	}
}

func TestParseEventsRejectsInvalidInput(t *testing.T) {
	cases := []string{
		"",
		`{"events":{}}`,
		`[{"pr":142}, "bad"]`,
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			_, err := ParseEvents([]byte(input), "input.json")
			if err == nil || err.Kind != ErrorArtifactContract {
				t.Fatalf("error = %#v, want artifact contract", err)
			}
		})
	}
}

func TestRedactForPersistenceRedactsKnownTokensAndSecretFields(t *testing.T) {
	got, err := RedactForPersistence(map[string]any{
		"message": "Authorization failed for Bearer ghp_1234567890abcdef1234567890abcdef123456",
		"metadata": map[string]any{
			"secrets": []any{"short-password", "low-token"},
		},
	}, "input.json")
	if err != nil {
		t.Fatalf("RedactForPersistence error: %v", err)
	}
	event := got.(map[string]any)
	message := event["message"].(string)
	if strings.Contains(message, "ghp_1234567890abcdef") || !strings.Contains(message, "[REDACTED:token]") {
		t.Fatalf("message = %q", message)
	}
	metadata := event["metadata"].(map[string]any)
	if metadata["secrets"] != "[REDACTED:secret]" {
		t.Fatalf("metadata.secrets = %#v", metadata["secrets"])
	}
}

func TestRedactForPersistenceFailsClosedForSecretShapedKey(t *testing.T) {
	_, err := RedactForPersistence(map[string]any{
		"metadata": map[string]any{
			"ghp_1234567890abcdef1234567890abcdef123456": "value",
		},
	}, "input.json")
	if err == nil || err.Kind != ErrorRedactionSafety || !strings.Contains(err.Message, "secret-shaped object key") {
		t.Fatalf("error = %#v, want redaction safety", err)
	}
}

func TestRedactForPersistenceFailsClosedForUnsafeGitHubURLPath(t *testing.T) {
	_, err := RedactForPersistence(map[string]any{
		"provenance_urls": []any{
			"https://github.com/acme/z6MvN2p9QxR4sT8aK3vY7bL0cD5eF1gH2jP9mQ4rS6tU/pull/1",
		},
	}, "input.json")
	if err == nil || err.Kind != ErrorRedactionSafety || !strings.Contains(err.Message, "high-entropy") {
		t.Fatalf("error = %#v, want redaction safety", err)
	}
}

func TestValidGitHubProvenanceURLShape(t *testing.T) {
	if !ValidGitHubProvenanceURLShape("https://github.com/acme/billing-service/pull/142") {
		t.Fatal("expected clean GitHub URL")
	}
	for _, value := range []string{
		"http://github.com/acme/billing-service/pull/142",
		"https://github.com/acme/billing-service/pull/142?token=abc",
		"https://github.com/acme/billing-service/pull/142#fragment",
		"https://token@github.com/acme/billing-service/pull/142",
	} {
		if ValidGitHubProvenanceURLShape(value) {
			t.Fatalf("URL %q should not be canonical", value)
		}
	}
}

func TestNormalizeRepoAcceptsStringAndFallbackFields(t *testing.T) {
	repo, ingestErr := NormalizeRepo(map[string]any{
		"repo": "Clyra-AI/relia",
	}, "input.json")
	if ingestErr != nil {
		t.Fatalf("NormalizeRepo returned error: %v", ingestErr)
	}
	if repo.Provider != "github" || repo.Owner != "Clyra-AI" || repo.Name != "relia" {
		t.Fatalf("repo = %#v", repo)
	}

	repo, ingestErr = NormalizeRepo(map[string]any{
		"repo_owner": "Clyra-AI",
		"repo_name":  "relia",
	}, "input.json")
	if ingestErr != nil {
		t.Fatalf("NormalizeRepo fallback returned error: %v", ingestErr)
	}
	if repo.Owner != "Clyra-AI" || repo.Name != "relia" {
		t.Fatalf("fallback repo = %#v", repo)
	}
}

func TestNormalizeRepoRejectsUnsupportedProvider(t *testing.T) {
	_, ingestErr := NormalizeRepo(map[string]any{
		"repo": map[string]any{
			"provider": "gitlab",
			"owner":    "Clyra-AI",
			"name":     "relia",
		},
	}, "input.json")

	if ingestErr == nil {
		t.Fatal("expected provider error")
	}
	if ingestErr.Message != "experience repo.provider must be github" {
		t.Fatalf("Message = %q", ingestErr.Message)
	}
}

func TestNormalizeRepoRequiresOwnerAndName(t *testing.T) {
	_, ingestErr := NormalizeRepo(map[string]any{"repo": "Clyra-AI"}, "input.json")

	if ingestErr == nil {
		t.Fatal("expected missing owner/name error")
	}
	if ingestErr.Message != "experience repo must include owner and name" {
		t.Fatalf("Message = %q", ingestErr.Message)
	}
}

func TestNormalizeActionAcceptsFlatAndNestedFields(t *testing.T) {
	cases := []struct {
		name  string
		event map[string]any
		want  Action
	}{
		{
			name: "flat",
			event: map[string]any{
				"pr":     122,
				"commit": "abc123",
			},
			want: Action{PR: 122, Commit: "abc123"},
		},
		{
			name: "nested with commits fallback",
			event: map[string]any{
				"action": map[string]any{
					"pr":      json.Number("123"),
					"commits": []any{"def456", "ignored"},
				},
			},
			want: Action{PR: 123, Commit: "def456"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ingestErr := NormalizeAction(tc.event, "input.json")
			if ingestErr != nil {
				t.Fatalf("NormalizeAction error: %v", ingestErr)
			}
			if got != tc.want {
				t.Fatalf("action = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestNormalizeActionRejectsInvalidPR(t *testing.T) {
	cases := []struct {
		name  string
		event map[string]any
	}{
		{name: "missing", event: map[string]any{"commit": "abc123"}},
		{name: "zero", event: map[string]any{"pr": 0, "commit": "abc123"}},
		{name: "fractional", event: map[string]any{"pr": 1.5, "commit": "abc123"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ingestErr := NormalizeAction(tc.event, "input.json")
			if ingestErr == nil || ingestErr.Kind != ErrorProvenance || !strings.Contains(ingestErr.Message, "experience record PR number") {
				t.Fatalf("error = %#v, want provenance PR error", ingestErr)
			}
		})
	}
}

func TestNormalizeActionRejectsMissingCommit(t *testing.T) {
	_, ingestErr := NormalizeAction(map[string]any{"pr": 122}, "input.json")

	if ingestErr == nil || ingestErr.Kind != ErrorArtifactContract || ingestErr.Message != "experience record must include commit" {
		t.Fatalf("error = %#v, want missing commit contract error", ingestErr)
	}
}

func TestPersistRecordsWritesSortedMonthlyShardAndUpserts(t *testing.T) {
	root := t.TempDir()
	records := []Record{
		{
			ObjectType:      "relia.experience_record",
			SchemaVersion:   "1.0",
			ExperienceID:    "exp_0002",
			Repo:            Repo{Provider: "github", Owner: "acme", Name: "billing"},
			RecordedAt:      "2026-04-02T10:00:00Z",
			Action:          Action{PR: 2, Commit: "abc2"},
			Outcome:         Outcome{Kind: "ci_failure", TerminalState: "failed", Signature: Signature{SignatureID: "sig_b", ExtractionConfidence: "structured"}},
			Provenance:      Provenance{URLs: []string{"https://github.com/acme/billing/pull/2"}},
			RedactionStatus: "applied",
			ShareScope:      "private",
			Metadata:        map[string]any{"value": "new"},
		},
		{
			ObjectType:      "relia.experience_record",
			SchemaVersion:   "1.0",
			ExperienceID:    "exp_0001",
			Repo:            Repo{Provider: "github", Owner: "acme", Name: "billing"},
			RecordedAt:      "2026-04-01T10:00:00Z",
			Action:          Action{PR: 1, Commit: "abc1"},
			Outcome:         Outcome{Kind: "ci_failure", TerminalState: "failed", Signature: Signature{SignatureID: "sig_a", ExtractionConfidence: "structured"}},
			Provenance:      Provenance{URLs: []string{"https://github.com/acme/billing/pull/1"}},
			RedactionStatus: "applied",
			ShareScope:      "private",
			Metadata:        map[string]any{"value": "new"},
		},
	}

	shards, err := PersistRecords(root, records)
	if err != nil {
		t.Fatalf("PersistRecords error: %v", err)
	}
	if len(shards) != 1 || shards[0] != ".relia/experiences/2026-04.jsonl" {
		t.Fatalf("shards = %#v", shards)
	}

	content, readErr := os.ReadFile(filepath.Join(root, ".relia", "experiences", "2026-04.jsonl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(content), `"experience_id":"exp_0001"`) ||
		!strings.Contains(string(content), `"experience_id":"exp_0002"`) {
		t.Fatalf("content missing records:\n%s", content)
	}
	if strings.Index(string(content), `"experience_id":"exp_0001"`) < strings.Index(string(content), `"experience_id":"exp_0002"`) {
		t.Fatalf("records were not preserved in write order:\n%s", content)
	}

	records[0].Metadata["value"] = "updated"
	if _, err := PersistRecords(root, records[:1]); err != nil {
		t.Fatalf("second PersistRecords error: %v", err)
	}
	updated, readErr := os.ReadFile(filepath.Join(root, ".relia", "experiences", "2026-04.jsonl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(updated), `"value":"updated"`) {
		t.Fatalf("existing record was not upserted:\n%s", updated)
	}
}

func TestPersistRecordsReportsCorruptShardAsProvenance(t *testing.T) {
	root := t.TempDir()
	shardPath := filepath.Join(root, ".relia", "experiences", "2026-04.jsonl")
	if err := os.MkdirAll(filepath.Dir(shardPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shardPath, []byte("{not-json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := PersistRecords(root, []Record{{ExperienceID: "exp_0001", RecordedAt: "2026-04-01T10:00:00Z"}})
	if err == nil || err.Kind != ErrorProvenance {
		t.Fatalf("error = %#v, want provenance", err)
	}
}

func TestGitHubPullRequestHelpers(t *testing.T) {
	record := Record{
		Repo:   Repo{Owner: "acme", Name: "billing"},
		Action: Action{PR: 142},
	}

	if !GitHubProvenanceURLRepoMatchesRecord("https://github.com/acme/billing/actions/runs/1", record) {
		t.Fatal("expected repo match")
	}
	if number, ok := GitHubPullRequestURLPathNumber("https://github.com/acme/billing/pull/142/files"); !ok || number != 142 {
		t.Fatalf("path PR = %d/%v", number, ok)
	}
	if number, ok := GitHubPullRequestURLNumber("https://github.com/acme/billing/pull/142"); !ok || number != 142 {
		t.Fatalf("PR = %d/%v", number, ok)
	}
	if !GitHubPullRequestURLMatchesRecord("https://github.com/acme/billing/pull/142", record) {
		t.Fatal("expected PR URL to match record")
	}
	if got := GitHubPullRequestURLForRecord(record); got != "https://github.com/acme/billing/pull/142" {
		t.Fatalf("URL = %q", got)
	}
	record.Provenance.URLs = []string{
		"https://github.com/acme/billing/actions/runs/99",
		"https://github.com/acme/billing/pull/142",
	}
	if got := PrimaryProvenanceURL(record); got != "https://github.com/acme/billing/pull/142" {
		t.Fatalf("primary URL = %q, want matching pull URL", got)
	}
	record.Provenance.URLs = []string{"https://github.com/acme/billing/actions/runs/99"}
	if got := PrimaryProvenanceURL(record); got != "https://github.com/acme/billing/pull/142" {
		t.Fatalf("primary URL = %q, want derived pull URL", got)
	}
	record.Action.PR = 0
	if got := PrimaryProvenanceURL(record); got != "https://github.com/acme/billing/actions/runs/99" {
		t.Fatalf("primary URL = %q, want first provenance URL fallback", got)
	}
}

func TestValidateRecordAcceptsCanonicalPrivateRecord(t *testing.T) {
	record := validRecordForTest()

	recordedAt, err := ValidateRecord(record, ".relia/experiences/2026-04.jsonl:1", "1.0")
	if err != nil {
		t.Fatalf("ValidateRecord error: %v", err)
	}
	if got := recordedAt.Format("2006-01-02T15:04:05Z"); got != "2026-04-02T18:21:00Z" {
		t.Fatalf("recordedAt = %s", got)
	}
}

func TestValidateRecordRejectsProvenanceRepoMismatch(t *testing.T) {
	record := validRecordForTest()
	record.Provenance.URLs = []string{"https://github.com/other/repo/pull/142"}

	_, err := ValidateRecord(record, "input.json:1", "1.0")
	if err == nil || err.Kind != ErrorProvenance || !strings.Contains(err.Message, "repo must match") {
		t.Fatalf("error = %#v, want provenance mismatch", err)
	}
}

func TestValidateRecordRejectsUnverifiedMetadataSource(t *testing.T) {
	record := validRecordForTest()
	record.Metadata = map[string]any{"source": "agent_reflection"}

	_, err := ValidateRecord(record, "input.json:1", "1.0")
	if err == nil || err.Kind != ErrorArtifactContract || !strings.Contains(err.Message, "self-reports") {
		t.Fatalf("error = %#v, want self-report rejection", err)
	}
}

func TestCanonicalDistillInputRecord(t *testing.T) {
	event := map[string]any{
		"object_type":      "relia.experience_record",
		"schema_version":   "1.0",
		"experience_id":    "exp_0142",
		"repo":             map[string]any{"provider": "github", "owner": "acme", "name": "billing"},
		"recorded_at":      "2026-04-02T18:21:00Z",
		"attribution":      map[string]any{"actor_kind": "agent", "method": "manual", "confidence": 1.0},
		"context":          map[string]any{"paths": []any{"cmd/relia/main.go"}, "diff_fingerprint": "abc123"},
		"action":           map[string]any{"pr": 142, "commit": "abc1234"},
		"outcome":          map[string]any{"kind": "ci_failure", "terminal_state": "failed", "signature": map[string]any{"signature_id": "sig_abc123", "extraction_confidence": "structured"}},
		"provenance":       map[string]any{"urls": []any{"https://github.com/acme/billing/pull/142"}},
		"flake_discount":   0.0,
		"org_eligible":     false,
		"share_scope":      "private",
		"redaction_status": "applied",
		"metadata":         map[string]any{},
	}

	record, canonical, err := CanonicalDistillInputRecord(event, "input.json:1")
	if err != nil {
		t.Fatalf("CanonicalDistillInputRecord error: %v", err)
	}
	if !canonical || record.ExperienceID != "exp_0142" || record.Action.PR != 142 {
		t.Fatalf("record = %#v canonical=%v", record, canonical)
	}
}

func TestCanonicalDistillInputRecordRejectsIncompleteCanonicalRecord(t *testing.T) {
	event := map[string]any{
		"object_type": "relia.experience_record",
		"action":      map[string]any{"commit": "abc1234"},
		"attribution": map[string]any{"method": "manual"},
		"context":     map[string]any{},
	}

	_, canonical, err := CanonicalDistillInputRecord(event, "input.json:1")
	if !canonical {
		t.Fatal("expected canonical input")
	}
	if err == nil || err.Kind != ErrorArtifactContract || !strings.Contains(err.Message, "context.diff_fingerprint") {
		t.Fatalf("error = %#v, want missing diff fingerprint", err)
	}
}

func validRecordForTest() Record {
	return Record{
		ObjectType:    "relia.experience_record",
		SchemaVersion: "1.0",
		ExperienceID:  "exp_0142",
		Repo:          Repo{Provider: "github", Owner: "acme", Name: "billing"},
		RecordedAt:    "2026-04-02T18:21:00Z",
		Attribution: Attribution{
			ActorKind:  "agent",
			Method:     "manual",
			Confidence: 1,
		},
		Context: Context{
			Paths:           []string{"cmd/relia/main.go"},
			DiffFingerprint: "abc123",
		},
		Action: Action{
			PR:     142,
			Commit: "abc1234",
		},
		Outcome: Outcome{
			Kind:          "ci_failure",
			TerminalState: "failed",
			Signature: Signature{
				SignatureID:          "sig_abc123",
				ExtractionConfidence: "structured",
			},
		},
		Provenance:      Provenance{URLs: []string{"https://github.com/acme/billing/pull/142"}},
		FlakeDiscount:   0,
		OrgEligible:     false,
		ShareScope:      "private",
		RedactionStatus: "applied",
		Metadata:        map[string]any{},
	}
}
