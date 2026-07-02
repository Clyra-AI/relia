package distill

import (
	"math"
	"time"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
)

func ConfidenceMetadata(records []backtestdoc.Experience, contradictions int, anchor time.Time, halfLifeDays int, flakeHeuristics map[string]string) (float64, RuleMetadata) {
	if len(records) == 0 {
		return 0, RuleMetadata{ConfidenceLabel: "low", HalfLifeDays: halfLifeDays}
	}
	oldest := records[0].RecordedAt.UTC()
	latest := records[0].RecordedAt.UTC()
	recencyTotal := 0.0
	extractionTotal := 0.0
	flakeTotal := 0.0
	for _, record := range records {
		if record.RecordedAt.Before(oldest) {
			oldest = record.RecordedAt.UTC()
		}
		if record.RecordedAt.After(latest) {
			latest = record.RecordedAt.UTC()
		}
		ageDays := anchor.Sub(record.RecordedAt.UTC()).Hours() / 24
		if ageDays < 0 {
			ageDays = 0
		}
		recencyTotal += math.Pow(0.5, ageDays/float64(halfLifeDays))
		extractionTotal += extractionConfidenceScore(record.Record.Outcome.Signature.ExtractionConfidence)
		flakeTotal += FlakeDiscount(record, flakeHeuristics)
	}
	count := float64(len(records))
	evidenceScore := math.Sqrt(count) / math.Sqrt(3)
	if evidenceScore > 1 {
		evidenceScore = 1
	}
	recencyWeight := recencyTotal / count
	extractionScore := extractionTotal / count
	flakeDiscount := flakeTotal / count
	flakeScore := 1 - flakeDiscount
	if flakeScore < 0 {
		flakeScore = 0
	}
	contradictionPenalty := 1 - math.Min(0.65, float64(contradictions)*0.25)
	confidence := roundFloat((0.40*evidenceScore+0.25*recencyWeight+0.20*extractionScore+0.15*flakeScore)*contradictionPenalty, 4)
	if len(records) < 3 && confidence > 0.6 {
		confidence = 0.6
	}
	label := confidenceLabel(confidence)
	return confidence, RuleMetadata{
		ConfidenceLabel:      label,
		EvidenceCount:        len(records),
		RecencyWeight:        roundFloat(recencyWeight, 4),
		Contradictions:       contradictions,
		FlakeDiscount:        roundFloat(flakeDiscount, 4),
		ExtractionConfidence: roundFloat(extractionScore, 4),
		DraftingModelWeight:  0,
		HalfLifeDays:         halfLifeDays,
		LatestEvidenceAt:     latest.Format(time.RFC3339),
		OldestEvidenceAt:     oldest.Format(time.RFC3339),
		AnchorRecordedAt:     anchor.UTC().Format(time.RFC3339),
	}
}

func extractionConfidenceScore(value string) float64 {
	switch value {
	case "structured":
		return 1
	case "log_parsed_high":
		return 0.85
	case "log_parsed_low":
		return 0.45
	default:
		return 0.2
	}
}

func confidenceLabel(confidence float64) string {
	switch {
	case confidence >= 0.75:
		return "high"
	case confidence >= 0.5:
		return "medium"
	default:
		return "low"
	}
}
