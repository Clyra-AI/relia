package distill

import (
	"testing"
	"time"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

func TestConfidenceMetadataBuildsStableInputs(t *testing.T) {
	anchor := mustParseTimeForConfidenceTest("2026-04-10T10:00:00Z")
	records := []backtestdoc.Experience{
		confidenceExperienceForTest("exp-1", "structured", "2026-04-10T10:00:00Z", 0),
		confidenceExperienceForTest("exp-2", "log_parsed_high", "2026-04-09T10:00:00Z", 0.2),
		confidenceExperienceForTest("exp-3", "log_parsed_low", "2026-04-08T10:00:00Z", 0),
	}

	confidence, metadata := ConfidenceMetadata(records, 1, anchor, 30, map[string]string{"exp-3": "duplicate support"})

	if confidence != 0.6658 {
		t.Fatalf("confidence = %.4f, want 0.6658", confidence)
	}
	if metadata.ConfidenceLabel != "medium" {
		t.Fatalf("confidence label = %q, want medium", metadata.ConfidenceLabel)
	}
	if metadata.EvidenceCount != 3 || metadata.Contradictions != 1 {
		t.Fatalf("metadata counts = %#v", metadata)
	}
	if metadata.RecencyWeight != 0.9773 || metadata.ExtractionConfidence != 0.7667 || metadata.FlakeDiscount != 0.4 {
		t.Fatalf("metadata scores = %#v", metadata)
	}
	if metadata.LatestEvidenceAt != "2026-04-10T10:00:00Z" || metadata.OldestEvidenceAt != "2026-04-08T10:00:00Z" || metadata.AnchorRecordedAt != "2026-04-10T10:00:00Z" {
		t.Fatalf("metadata timestamps = %#v", metadata)
	}
}

func TestConfidenceMetadataCapsSmallEvidenceSets(t *testing.T) {
	anchor := mustParseTimeForConfidenceTest("2026-04-10T10:00:00Z")
	records := []backtestdoc.Experience{
		confidenceExperienceForTest("exp-1", "structured", "2026-04-10T10:00:00Z", 0),
		confidenceExperienceForTest("exp-2", "structured", "2026-04-10T10:00:00Z", 0),
	}

	confidence, metadata := ConfidenceMetadata(records, 0, anchor, 30, nil)

	if confidence != 0.6 {
		t.Fatalf("confidence = %.4f, want small-evidence cap 0.6", confidence)
	}
	if metadata.ConfidenceLabel != "medium" {
		t.Fatalf("confidence label = %q, want medium", metadata.ConfidenceLabel)
	}
}

func TestConfidenceMetadataHandlesEmptyEvidence(t *testing.T) {
	confidence, metadata := ConfidenceMetadata(nil, 0, time.Time{}, 90, nil)

	if confidence != 0 {
		t.Fatalf("confidence = %.4f, want 0", confidence)
	}
	if metadata.ConfidenceLabel != "low" || metadata.HalfLifeDays != 90 {
		t.Fatalf("metadata = %#v, want low label and half-life preserved", metadata)
	}
}

func confidenceExperienceForTest(id string, extractionConfidence string, recordedAt string, flakeDiscount float64) backtestdoc.Experience {
	return backtestdoc.Experience{
		RecordedAt: mustParseTimeForConfidenceTest(recordedAt),
		Record: ingestdoc.Record{
			ExperienceID:  id,
			FlakeDiscount: flakeDiscount,
			Outcome: ingestdoc.Outcome{
				Signature: ingestdoc.Signature{ExtractionConfidence: extractionConfidence},
			},
		},
	}
}

func mustParseTimeForConfidenceTest(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
