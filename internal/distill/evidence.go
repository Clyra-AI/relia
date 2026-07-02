package distill

import backtestdoc "github.com/Clyra-AI/relia/internal/backtest"

func FailureEvidence(records []backtestdoc.Experience) []backtestdoc.Experience {
	var evidence []backtestdoc.Experience
	for _, record := range records {
		if isFailureOutcome(record.Record.Outcome.Kind) {
			evidence = append(evidence, record)
		}
	}
	return evidence
}

func PositiveEvidence(records []backtestdoc.Experience) []backtestdoc.Experience {
	var evidence []backtestdoc.Experience
	for _, record := range records {
		switch record.Record.Outcome.Kind {
		case "fix_held", "merged_clean":
			evidence = append(evidence, record)
		}
	}
	return evidence
}

func HeldEvidence(records []backtestdoc.Experience) []backtestdoc.Experience {
	var evidence []backtestdoc.Experience
	for _, record := range records {
		if record.Record.Outcome.Kind == "fix_held" {
			evidence = append(evidence, record)
		}
	}
	return evidence
}

func AllEvidenceDiscounted(records []backtestdoc.Experience, flakeHeuristics map[string]string) bool {
	if len(records) == 0 {
		return true
	}
	for _, record := range records {
		if FlakeDiscount(record, flakeHeuristics) < 0.75 {
			return false
		}
	}
	return true
}

func PlaybookContradictions(positives []backtestdoc.Experience, failures []backtestdoc.Experience) int {
	if len(positives) == 0 || len(failures) == 0 {
		return 0
	}
	latestPositive := positives[0].RecordedAt
	for _, record := range positives[1:] {
		if record.RecordedAt.After(latestPositive) {
			latestPositive = record.RecordedAt
		}
	}
	contradictions := 0
	for _, failure := range failures {
		if failure.RecordedAt.After(latestPositive) {
			contradictions++
		}
	}
	return contradictions
}

func AvoidContradictions(failures []backtestdoc.Experience, positives []backtestdoc.Experience) int {
	if len(failures) == 0 || len(positives) == 0 {
		return 0
	}
	latestFailure := failures[0].RecordedAt
	for _, failure := range failures[1:] {
		if failure.RecordedAt.After(latestFailure) {
			latestFailure = failure.RecordedAt
		}
	}
	contradictions := 0
	for _, positive := range positives {
		if positive.RecordedAt.After(latestFailure) {
			contradictions++
		}
	}
	return contradictions
}

func FlakeDiscount(record backtestdoc.Experience, flakeHeuristics map[string]string) float64 {
	discount := record.Record.FlakeDiscount
	if flakeHeuristics[record.Record.ExperienceID] != "" && discount < 1 {
		discount = 1
	}
	if discount < 0 {
		return 0
	}
	if discount > 1 {
		return 1
	}
	return discount
}

func isFailureOutcome(kind string) bool {
	switch kind {
	case "ci_failure", "revert", "review_correction":
		return true
	default:
		return false
	}
}
