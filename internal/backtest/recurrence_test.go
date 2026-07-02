package backtest

import (
	"reflect"
	"testing"
	"time"

	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

func TestRecurrenceSignatureKeysUseMetadataBeforeIDFallback(t *testing.T) {
	record := recurrenceRecordForTest("exp-1", 1, "sig-id", []string{"cmd/app.go"})
	record.Metadata = map[string]any{
		"signature": map[string]any{
			"class":               "build",
			"key":                 "missing-symbol",
			"message_fingerprint": "abc123",
		},
	}

	got := RecurrenceSignatureKeys(record)
	want := []string{"class_key\x00build\x00missing-symbol", "message\x00abc123"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RecurrenceSignatureKeys = %#v, want %#v", got, want)
	}
}

func TestSelectRecurrencePriorPrefersNewestConfirmedDifferentPR(t *testing.T) {
	oldPrior := recurrenceExperienceForTest("exp-1", 1, "sig-a", []string{"cmd/app.go"}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	newPrior := recurrenceExperienceForTest("exp-2", 2, "sig-a", []string{"cmd/app.go"}, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	samePR := recurrenceExperienceForTest("exp-3", 4, "sig-a", []string{"cmd/app.go"}, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	current := recurrenceExperienceForTest("exp-4", 4, "sig-a", []string{"cmd/app.go"}, time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC))

	prior, confidence, ok := SelectRecurrencePrior([]Experience{oldPrior, newPrior, samePR}, current)

	if !ok || confidence != "confirmed" || prior.Record.ExperienceID != "exp-2" {
		t.Fatalf("SelectRecurrencePrior = (%#v, %q, %v), want exp-2 confirmed", prior, confidence, ok)
	}
}

func TestBuildRecurrencePairUsesMatchedMessageSignatureAndRefs(t *testing.T) {
	prior := recurrenceExperienceForTest("exp-1", 11, "sig-old", []string{"internal/service.go"}, time.Time{})
	current := recurrenceExperienceForTest("exp-2", 12, "sig-new", []string{"internal/service.go"}, time.Time{})
	prior.Record.Metadata["signature"].(map[string]any)["message_fingerprint"] = "panic-abc"
	current.Record.Metadata["signature"].(map[string]any)["message_fingerprint"] = "panic-abc"

	pair := BuildRecurrencePair(prior, current)

	if pair.MatchedSignatureID != "message:panic-abc" {
		t.Fatalf("MatchedSignatureID = %q, want message fingerprint", pair.MatchedSignatureID)
	}
	if pair.PriorURL != "https://github.com/acme/relia-test/pull/11" || pair.CurrentURL != "https://github.com/acme/relia-test/pull/12" {
		t.Fatalf("urls = %q %q, want derived PR URLs", pair.PriorURL, pair.CurrentURL)
	}
	if !reflect.DeepEqual(pair.Refs, []string{"events.jsonl:11", "events.jsonl:12"}) {
		t.Fatalf("refs = %#v, want source line refs", pair.Refs)
	}
}

func TestRecurrencePriorCandidatesDeduplicatesAndSorts(t *testing.T) {
	first := recurrenceExperienceForTest("exp-1", 1, "sig-a", []string{"a.go"}, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	duplicate := first
	later := recurrenceExperienceForTest("exp-2", 2, "sig-a", []string{"a.go"}, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	index := map[string][]Experience{
		"sig": {later, first, duplicate},
	}

	got := RecurrencePriorCandidates(index, []string{"sig"})

	if len(got) != 2 || got[0].Record.ExperienceID != "exp-1" || got[1].Record.ExperienceID != "exp-2" {
		t.Fatalf("RecurrencePriorCandidates = %#v, want deduplicated chronological priors", got)
	}
}

func recurrenceExperienceForTest(id string, pr int, signatureID string, paths []string, recordedAt time.Time) Experience {
	return Experience{
		Record:     recurrenceRecordForTest(id, pr, signatureID, paths),
		RecordedAt: recordedAt,
		SourcePath: "events.jsonl",
		SourceLine: pr,
	}
}

func recurrenceRecordForTest(id string, pr int, signatureID string, paths []string) ingestdoc.Record {
	return ingestdoc.Record{
		ExperienceID: id,
		Repo: ingestdoc.Repo{
			Provider: "github",
			Owner:    "acme",
			Name:     "relia-test",
		},
		Attribution: ingestdoc.Attribution{
			ActorKind: "agent",
		},
		Context: ingestdoc.Context{
			Paths: paths,
		},
		Action: ingestdoc.Action{
			PR: pr,
		},
		Outcome: ingestdoc.Outcome{
			Kind: "ci_failure",
			Signature: ingestdoc.Signature{
				SignatureID:          signatureID,
				ExtractionConfidence: "structured",
			},
		},
		Metadata: map[string]any{
			"signature": map[string]any{
				"class": "build",
				"key":   signatureID,
			},
		},
	}
}
