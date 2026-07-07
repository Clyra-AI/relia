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
		"paths": []string{
			"internal/ghp_1234567890abcdef1234567890abcdef123456/main.go",
		},
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
	paths := event["paths"].([]string)
	if len(paths) != 1 || strings.Contains(paths[0], "ghp_1234567890abcdef") || !strings.Contains(paths[0], "[REDACTED:token]") {
		t.Fatalf("paths = %#v, want redacted token path", paths)
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

func TestRedactForPersistenceAllowsCanonicalGitHubDetailsURL(t *testing.T) {
	_, err := RedactForPersistence(map[string]any{
		"details_url": "https://github.com/acme/billing-service/actions/runs/1234567890/job/9876543210",
	}, "input.json")
	if err != nil {
		t.Fatalf("error = %#v, want canonical GitHub details URL allowed", err)
	}
}

func TestRedactForPersistenceFailsClosedForUnsafeGitHubDetailsURL(t *testing.T) {
	_, err := RedactForPersistence(map[string]any{
		"details_url": "https://github.com/acme/billing-service/actions/runs/z6MvN2p9QxR4sT8aK3vY7bL0cD5eF1gH2jP9mQ4rS6tU",
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

func TestNormalizeProvenanceCollectsAndDeduplicatesURLs(t *testing.T) {
	got, ingestErr := NormalizeProvenance(map[string]any{
		"provenance_urls": []any{
			"https://github.com/acme/billing/pull/122",
			"https://github.com/acme/billing/pull/122",
		},
		"provenance": map[string]any{
			"check_run_url": "https://github.com/acme/billing/actions/runs/123",
			"revert_url":    "https://github.com/acme/billing/pull/123",
		},
	}, "input.json")

	if ingestErr != nil {
		t.Fatalf("NormalizeProvenance error: %v", ingestErr)
	}
	want := []string{
		"https://github.com/acme/billing/pull/122",
		"https://github.com/acme/billing/actions/runs/123",
		"https://github.com/acme/billing/pull/123",
	}
	if strings.Join(got.URLs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("URLs = %#v, want %#v", got.URLs, want)
	}
}

func TestNormalizeProvenanceRejectsMissingURLs(t *testing.T) {
	_, ingestErr := NormalizeProvenance(map[string]any{}, "input.json")

	if ingestErr == nil || ingestErr.Kind != ErrorProvenance || ingestErr.Message != "experience record must include at least one provenance URL" {
		t.Fatalf("error = %#v, want missing provenance error", ingestErr)
	}
}

func TestNormalizeProvenanceRejectsInvalidURLShape(t *testing.T) {
	_, ingestErr := NormalizeProvenance(map[string]any{
		"provenance_urls": []any{"https://github.com/acme/billing/pull/122?token=abc"},
	}, "input.json")

	if ingestErr == nil || ingestErr.Kind != ErrorProvenance || ingestErr.Message != "experience provenance URL must be a canonical https://github.com/ URL" {
		t.Fatalf("error = %#v, want invalid URL provenance error", ingestErr)
	}
}

func TestNormalizeContextAcceptsExplicitDiffFingerprint(t *testing.T) {
	got, ingestErr := NormalizeContext(map[string]any{
		"paths":            []any{"cmd/relia/main.go"},
		"diff_fingerprint": "sha256:explicit",
	}, Action{PR: 124, Commit: "abc123"}, "input.json")

	if ingestErr != nil {
		t.Fatalf("NormalizeContext error: %v", ingestErr)
	}
	if len(got.Paths) != 1 || got.Paths[0] != "cmd/relia/main.go" {
		t.Fatalf("Paths = %#v", got.Paths)
	}
	if got.DiffFingerprint != "sha256:explicit" {
		t.Fatalf("DiffFingerprint = %q", got.DiffFingerprint)
	}
}

func TestNormalizeContextGeneratesDiffFingerprint(t *testing.T) {
	got, ingestErr := NormalizeContext(map[string]any{
		"context": map[string]any{
			"paths": []any{"cmd/relia/main.go", "internal/ingest/record_validation.go"},
		},
	}, Action{PR: 124, Commit: "abc123"}, "input.json")

	if ingestErr != nil {
		t.Fatalf("NormalizeContext error: %v", ingestErr)
	}
	want := sha256String("124|abc123|cmd/relia/main.go|internal/ingest/record_validation.go")
	if got.DiffFingerprint != want {
		t.Fatalf("DiffFingerprint = %q, want %q", got.DiffFingerprint, want)
	}
}

func TestNormalizeContextRejectsMissingPaths(t *testing.T) {
	_, ingestErr := NormalizeContext(map[string]any{}, Action{PR: 124, Commit: "abc123"}, "input.json")

	if ingestErr == nil || ingestErr.Kind != ErrorArtifactContract || ingestErr.Message != "experience record must include at least one context path" {
		t.Fatalf("error = %#v, want missing context path error", ingestErr)
	}
}

func TestNormalizeAttributionAcceptsExplicitHuman(t *testing.T) {
	got, skipped, ingestErr := NormalizeAttribution(map[string]any{
		"attribution": map[string]any{
			"actor_kind": "human",
		},
	}, AttributionPolicy{}, "input.json")

	if ingestErr != nil {
		t.Fatalf("NormalizeAttribution error: %v", ingestErr)
	}
	if skipped {
		t.Fatal("explicit human attribution should not skip")
	}
	if got.ActorKind != "human" || got.Method != "manual" || got.Confidence != 1 {
		t.Fatalf("attribution = %#v", got)
	}
}

func TestNormalizeAttributionInfersAgentFromPolicy(t *testing.T) {
	cases := []struct {
		name   string
		event  map[string]any
		policy AttributionPolicy
		method string
	}{
		{
			name: "label",
			event: map[string]any{
				"labels": []any{"relia-agent"},
			},
			policy: AttributionPolicy{PRLabels: []string{"RELIA-AGENT"}},
			method: "pr_label",
		},
		{
			name: "coauthor",
			event: map[string]any{
				"coauthors": []any{"relia-bot"},
			},
			policy: AttributionPolicy{CoauthorTrailers: []string{"relia-bot"}},
			method: "coauthor_trailer",
		},
		{
			name: "author login",
			event: map[string]any{
				"actor": map[string]any{"login": "relia-bot"},
			},
			policy: AttributionPolicy{AgentAuthorLogins: []string{"relia-bot"}},
			method: "bot_login",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, skipped, ingestErr := NormalizeAttribution(tc.event, tc.policy, "input.json")
			if ingestErr != nil {
				t.Fatalf("NormalizeAttribution error: %v", ingestErr)
			}
			if skipped {
				t.Fatal("agent attribution should not skip")
			}
			if got.ActorKind != "agent" || got.Method != tc.method || got.Confidence != 0.9 {
				t.Fatalf("attribution = %#v, want agent/%s/0.9", got, tc.method)
			}
		})
	}
}

func TestNormalizeAttributionSkipsUncertainByDefault(t *testing.T) {
	_, skipped, ingestErr := NormalizeAttribution(map[string]any{}, AttributionPolicy{}, "input.json")

	if ingestErr != nil {
		t.Fatalf("NormalizeAttribution error: %v", ingestErr)
	}
	if !skipped {
		t.Fatal("uncertain attribution should skip by default")
	}
}

func TestNormalizeAttributionIncludesUncertainWhenConfigured(t *testing.T) {
	got, skipped, ingestErr := NormalizeAttribution(map[string]any{}, AttributionPolicy{Uncertain: "include_flagged"}, "input.json")

	if ingestErr != nil {
		t.Fatalf("NormalizeAttribution error: %v", ingestErr)
	}
	if skipped {
		t.Fatal("include_flagged uncertain attribution should not skip")
	}
	if got.ActorKind != "uncertain" || got.Method != "uncertain" || got.Confidence != 0 {
		t.Fatalf("attribution = %#v", got)
	}
}

func TestNormalizeAttributionRejectsInvalidFields(t *testing.T) {
	cases := []struct {
		name  string
		event map[string]any
		want  string
	}{
		{name: "actor", event: map[string]any{"actor_kind": "service"}, want: "attribution actor_kind must be agent, human, or uncertain"},
		{name: "method", event: map[string]any{"actor_kind": "agent", "attribution_method": "guess"}, want: "attribution method is invalid"},
		{name: "confidence type", event: map[string]any{"actor_kind": "agent", "attribution_confidence": "high"}, want: "attribution confidence must be numeric"},
		{name: "confidence range", event: map[string]any{"actor_kind": "agent", "attribution_confidence": 1.5}, want: "attribution confidence must be between 0 and 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, ingestErr := NormalizeAttribution(tc.event, AttributionPolicy{}, "input.json")
			if ingestErr == nil || ingestErr.Kind != ErrorArtifactContract || ingestErr.Message != tc.want {
				t.Fatalf("error = %#v, want %q", ingestErr, tc.want)
			}
		})
	}
}

func TestNormalizeRecordBuildsCanonicalRecord(t *testing.T) {
	got, skipped, ingestErr := NormalizeRecord(map[string]any{
		"repo": map[string]any{
			"provider": "github",
			"owner":    "acme",
			"name":     "billing-service",
		},
		"recorded_at":            "2026-04-02T18:21:00Z",
		"pr":                     json.Number("142"),
		"commit":                 "abc1234",
		"paths":                  []any{"packages/billing/invoice.py"},
		"actor_kind":             "agent",
		"attribution_method":     "pr_label",
		"attribution_confidence": json.Number("0.9"),
		"outcome_kind":           "ci_failure",
		"signature_class":        "test_failure",
		"check_name":             "pytest-billing",
		"signature_key":          "tests/test_invoice.py::test_tz_rollover",
		"extraction_confidence":  "structured",
		"message":                "timezone rollover failed",
		"provenance_urls":        []any{"https://github.com/acme/billing-service/pull/142"},
		"flake_discount":         json.Number("0.25"),
		"metadata":               map[string]any{"source": "fixture"},
	}, RecordOptions{SchemaVersion: "1.0", SourceIndex: 3}, "input.json:1")

	if ingestErr != nil {
		t.Fatalf("NormalizeRecord error: %v", ingestErr)
	}
	if skipped {
		t.Fatal("NormalizeRecord skipped explicit agent attribution")
	}
	if got.ObjectType != "relia.experience_record" || got.SchemaVersion != "1.0" {
		t.Fatalf("record envelope = %#v", got)
	}
	if !strings.HasPrefix(got.ExperienceID, "exp_0142_") {
		t.Fatalf("generated experience_id = %q", got.ExperienceID)
	}
	if got.RecordedAt != "2026-04-02T18:21:00Z" || got.Action.PR != 142 || got.Action.Commit != "abc1234" {
		t.Fatalf("record action/time = %#v", got)
	}
	if got.FlakeDiscount != 0.25 || got.ShareScope != "private" || got.RedactionStatus != "applied" || got.OrgEligible {
		t.Fatalf("record safety/defaults = %#v", got)
	}
	if got.Metadata["source_input_index"] != 3 || got.Metadata["source_kind"] != "local_input" || got.Metadata["memory_source"] != "verified_outcome_event" {
		t.Fatalf("record metadata = %#v", got.Metadata)
	}
	signature, ok := got.Metadata["signature"].(map[string]any)
	if !ok || signature["message_fingerprint"] == "" || signature["class"] != "test_failure" {
		t.Fatalf("signature metadata = %#v", got.Metadata["signature"])
	}
}

func TestNormalizeRecordSkipsUncertainAttribution(t *testing.T) {
	_, skipped, ingestErr := NormalizeRecord(map[string]any{
		"repo":            "acme/billing-service",
		"recorded_at":     "2026-04-02T18:21:00Z",
		"pr":              142,
		"commit":          "abc1234",
		"paths":           []any{"packages/billing/invoice.py"},
		"outcome_kind":    "merged_clean",
		"provenance_urls": []any{"https://github.com/acme/billing-service/pull/142"},
	}, RecordOptions{SchemaVersion: "1.0"}, "input.json:1")

	if ingestErr != nil {
		t.Fatalf("NormalizeRecord error: %v", ingestErr)
	}
	if !skipped {
		t.Fatal("NormalizeRecord should skip uncertain attribution by default")
	}
}

func TestNormalizeOutcomeDefaultsSignatureFields(t *testing.T) {
	got, metadata, ingestErr := NormalizeOutcome(map[string]any{
		"outcome_kind": "ci_failure",
		"message":      "unit test failed",
	}, Action{PR: 125, Commit: "abc123"}, []string{"cmd/relia/main.go"}, "input.json")

	if ingestErr != nil {
		t.Fatalf("NormalizeOutcome error: %v", ingestErr)
	}
	if got.Kind != "ci_failure" || got.TerminalState != "failed" {
		t.Fatalf("outcome = %#v", got)
	}
	wantSignatureID := "sig_" + shortHash("test_failure|ci_failure|cmd/relia/main.go")
	if got.Signature.SignatureID != wantSignatureID || got.Signature.ExtractionConfidence != "structured" {
		t.Fatalf("signature = %#v, want id %q structured", got.Signature, wantSignatureID)
	}
	if metadata["class"] != "test_failure" ||
		metadata["check_name"] != "ci_failure" ||
		metadata["key"] != "cmd/relia/main.go" ||
		metadata["extraction_method"] != "structured_check_run" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata["message_fingerprint"] != sha256String("unit test failed") {
		t.Fatalf("message_fingerprint = %#v", metadata["message_fingerprint"])
	}
}

func TestNormalizeOutcomeAcceptsExplicitFields(t *testing.T) {
	got, metadata, ingestErr := NormalizeOutcome(map[string]any{
		"outcome": map[string]any{
			"kind":           "review_correction",
			"terminal_state": "corrected",
			"signature": map[string]any{
				"class":                 "review_correction",
				"check_name":            "codex",
				"key":                   "internal/ingest/record_validation.go",
				"signature_id":          "sig_explicit",
				"extraction_confidence": "log_parsed_low",
				"message_fingerprint":   "sha256:explicit",
			},
		},
	}, Action{PR: 125, Commit: "abc123"}, nil, "input.json")

	if ingestErr != nil {
		t.Fatalf("NormalizeOutcome error: %v", ingestErr)
	}
	if got.Signature.SignatureID != "sig_explicit" || got.Signature.ExtractionConfidence != "log_parsed_low" {
		t.Fatalf("signature = %#v", got.Signature)
	}
	if metadata["extraction_method"] != "log_parse" || metadata["message_fingerprint"] != "sha256:explicit" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestNormalizeOutcomeRejectsInvalidFields(t *testing.T) {
	cases := []struct {
		name  string
		event map[string]any
		want  string
	}{
		{name: "kind", event: map[string]any{"outcome_kind": "bad"}, want: "outcome kind is invalid"},
		{name: "terminal", event: map[string]any{"outcome_kind": "ci_failure", "terminal_state": "bad"}, want: "outcome terminal_state is invalid"},
		{name: "confidence", event: map[string]any{"outcome_kind": "ci_failure", "extraction_confidence": "bad"}, want: "signature extraction_confidence is invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, ingestErr := NormalizeOutcome(tc.event, Action{PR: 125, Commit: "abc123"}, nil, "input.json")
			if ingestErr == nil || ingestErr.Kind != ErrorArtifactContract || ingestErr.Message != tc.want {
				t.Fatalf("error = %#v, want %q", ingestErr, tc.want)
			}
		})
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
