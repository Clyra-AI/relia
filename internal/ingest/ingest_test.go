package ingest

import (
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
