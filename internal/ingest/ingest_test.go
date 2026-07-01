package ingest

import (
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
}
