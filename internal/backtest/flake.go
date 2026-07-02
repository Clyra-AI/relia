package backtest

import "sort"

func IsFlakeDiscounted(record Experience, heuristics map[string]string) bool {
	return record.Record.FlakeDiscount > 0 || heuristics[record.Record.ExperienceID] != ""
}

func BuildFlakeDiscount(record Experience, records []Experience, heuristics map[string]string) FlakeDiscount {
	supportingPRs := []int{}
	supportingRefs := []string{}
	for _, candidate := range records {
		if candidate.Record.ExperienceID == record.Record.ExperienceID {
			continue
		}
		if !RecordsShareRecurrenceSignature(candidate.Record, record.Record) {
			continue
		}
		if candidate.Record.Attribution.ActorKind != "agent" || !IsFailureOutcome(candidate.Record.Outcome.Kind) {
			continue
		}
		supportingPRs = append(supportingPRs, candidate.Record.Action.PR)
		supportingRefs = append(supportingRefs, SourceLineRef(candidate))
	}
	sort.Ints(supportingPRs)
	supportingRefs = uniqueStrings(supportingRefs)
	reason := heuristics[record.Record.ExperienceID]
	flakeDiscount := record.Record.FlakeDiscount
	if reason == "" {
		reason = "Discounted as flaky because the experience record carries an explicit flake_discount."
	}
	if flakeDiscount == 0 {
		flakeDiscount = 1
	}
	return FlakeDiscount{
		ExperienceID:    record.Record.ExperienceID,
		PR:              record.Record.Action.PR,
		SignatureID:     record.Record.Outcome.Signature.SignatureID,
		FlakeDiscount:   roundFloat(flakeDiscount, 4),
		SupportingPRs:   supportingPRs,
		SupportingRefs:  supportingRefs,
		Reason:          reason,
		ExcludedFromERR: true,
	}
}
