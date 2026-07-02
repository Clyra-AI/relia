package backtest

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

func IsFailureOutcome(kind string) bool {
	switch kind {
	case "ci_failure", "revert", "review_correction":
		return true
	default:
		return false
	}
}

func RecurrenceSignatureKeys(record ingestdoc.Record) []string {
	signatureMetadata, _ := record.Metadata["signature"].(map[string]any)
	signatureClass := strings.TrimSpace(stringFromAny(signatureMetadata["class"]))
	signatureKey := strings.TrimSpace(stringFromAny(signatureMetadata["key"]))
	messageFingerprint := strings.TrimSpace(stringFromAny(signatureMetadata["message_fingerprint"]))
	keys := []string{}
	if signatureClass != "" && signatureKey != "" {
		keys = append(keys, strings.Join([]string{"class_key", signatureClass, signatureKey}, "\x00"))
	}
	if messageFingerprint != "" {
		keys = append(keys, strings.Join([]string{"message", messageFingerprint}, "\x00"))
	}
	if len(keys) == 0 {
		keys = append(keys, strings.Join([]string{"id", record.Outcome.Signature.SignatureID}, "\x00"))
	}
	return keys
}

func MatchedRecurrenceSignatureID(left ingestdoc.Record, right ingestdoc.Record) string {
	leftSignatureID := strings.TrimSpace(left.Outcome.Signature.SignatureID)
	rightSignatureID := strings.TrimSpace(right.Outcome.Signature.SignatureID)
	rightKeys := map[string]bool{}
	for _, key := range RecurrenceSignatureKeys(right) {
		rightKeys[key] = true
	}
	for _, key := range RecurrenceSignatureKeys(left) {
		if rightKeys[key] {
			return displayRecurrenceSignatureKey(key, rightSignatureID)
		}
	}
	if leftSignatureID != "" && leftSignatureID == rightSignatureID {
		return rightSignatureID
	}
	if rightSignatureID != "" {
		return rightSignatureID
	}
	return leftSignatureID
}

func displayRecurrenceSignatureKey(key string, fallback string) string {
	parts := strings.Split(key, "\x00")
	switch {
	case len(parts) == 3 && parts[0] == "class_key":
		return strings.Join([]string{"class_key", parts[1], parts[2]}, ":")
	case len(parts) == 2 && parts[0] == "message":
		return "message:" + parts[1]
	case len(parts) == 2 && parts[0] == "id":
		return parts[1]
	case fallback != "":
		return fallback
	default:
		return strings.ReplaceAll(key, "\x00", ":")
	}
}

func AppendRecurrencePrior(priorBySignature map[string][]Experience, keys []string, current Experience) {
	for _, key := range keys {
		priorBySignature[key] = append(priorBySignature[key], current)
	}
}

func RecurrencePriorCandidates(priorBySignature map[string][]Experience, keys []string) []Experience {
	seen := map[string]bool{}
	priors := []Experience{}
	for _, key := range keys {
		for _, prior := range priorBySignature[key] {
			experienceID := prior.Record.ExperienceID
			if experienceID == "" {
				experienceID = SourceLineRef(prior)
			}
			if seen[experienceID] {
				continue
			}
			seen[experienceID] = true
			priors = append(priors, prior)
		}
	}
	sort.Slice(priors, func(i, j int) bool {
		if priors[i].RecordedAt.Equal(priors[j].RecordedAt) {
			return priors[i].Record.ExperienceID < priors[j].Record.ExperienceID
		}
		return priors[i].RecordedAt.Before(priors[j].RecordedAt)
	})
	return priors
}

func RecordsShareRecurrenceSignature(left ingestdoc.Record, right ingestdoc.Record) bool {
	rightKeys := map[string]bool{}
	for _, key := range RecurrenceSignatureKeys(right) {
		rightKeys[key] = true
	}
	for _, key := range RecurrenceSignatureKeys(left) {
		if rightKeys[key] {
			return true
		}
	}
	return false
}

func ConfirmedRecurrence(prior Experience, current Experience) bool {
	if prior.Record.Action.PR == current.Record.Action.PR {
		return false
	}
	if prior.Record.Outcome.Signature.SignatureID == "" ||
		!RecordsShareRecurrenceSignature(prior.Record, current.Record) {
		return false
	}
	if !reliableSignatureExtraction(prior.Record.Outcome.Signature.ExtractionConfidence) ||
		!reliableSignatureExtraction(current.Record.Outcome.Signature.ExtractionConfidence) {
		return false
	}
	return pathSetsOverlap(prior.Record.Context.Paths, current.Record.Context.Paths)
}

func reliableSignatureExtraction(value string) bool {
	return value == "structured" || value == "log_parsed_high"
}

func SelectRecurrencePrior(priors []Experience, current Experience) (Experience, string, bool) {
	for index := len(priors) - 1; index >= 0; index-- {
		if priors[index].Record.Action.PR == current.Record.Action.PR {
			continue
		}
		if ConfirmedRecurrence(priors[index], current) {
			return priors[index], "confirmed", true
		}
	}
	for index := len(priors) - 1; index >= 0; index-- {
		if priors[index].Record.Action.PR != current.Record.Action.PR {
			return priors[index], "possible", true
		}
	}
	return Experience{}, "", false
}

func pathSetsOverlap(left []string, right []string) bool {
	leftSet := map[string]bool{}
	for _, value := range left {
		if clean, ok := configdoc.CleanRepoPath(value); ok {
			leftSet[filepath.ToSlash(clean)] = true
		}
	}
	for _, value := range right {
		if clean, ok := configdoc.CleanRepoPath(value); ok && leftSet[filepath.ToSlash(clean)] {
			return true
		}
	}
	return false
}

func BuildRecurrencePair(prior Experience, current Experience) RecurrencePair {
	return RecurrencePair{
		CurrentExperienceID: current.Record.ExperienceID,
		PriorExperienceID:   prior.Record.ExperienceID,
		CurrentPR:           current.Record.Action.PR,
		PriorPR:             prior.Record.Action.PR,
		CurrentURL:          ingestdoc.PrimaryProvenanceURL(current.Record),
		PriorURL:            ingestdoc.PrimaryProvenanceURL(prior.Record),
		SignatureID:         current.Record.Outcome.Signature.SignatureID,
		MatchedSignatureID:  MatchedRecurrenceSignatureID(prior.Record, current.Record),
		Refs:                []string{SourceLineRef(prior), SourceLineRef(current)},
	}
}

func AddCitation(citations map[int]Citation, record Experience) {
	url := ingestdoc.PrimaryProvenanceURL(record.Record)
	if url == "" {
		return
	}
	citations[record.Record.Action.PR] = Citation{
		PR:           record.Record.Action.PR,
		URL:          url,
		ExperienceID: record.Record.ExperienceID,
	}
}

func SourceLineRef(record Experience) string {
	return fmt.Sprintf("%s:%d", record.SourcePath, record.SourceLine)
}

func SortRecurrencePairs(pairs []RecurrencePair) {
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].CurrentPR == pairs[j].CurrentPR {
			return pairs[i].CurrentExperienceID < pairs[j].CurrentExperienceID
		}
		return pairs[i].CurrentPR < pairs[j].CurrentPR
	})
}

func SortFlakeDiscounts(flakes []FlakeDiscount) {
	sort.Slice(flakes, func(i, j int) bool {
		if flakes[i].PR == flakes[j].PR {
			return flakes[i].ExperienceID < flakes[j].ExperienceID
		}
		return flakes[i].PR < flakes[j].PR
	})
}

func SortUncertain(uncertain []Uncertain) {
	sort.Slice(uncertain, func(i, j int) bool {
		if uncertain[i].PR == uncertain[j].PR {
			return uncertain[i].ExperienceID < uncertain[j].ExperienceID
		}
		return uncertain[i].PR < uncertain[j].PR
	})
}

func Citations(citationMap map[int]Citation) []Citation {
	citations := make([]Citation, 0, len(citationMap))
	for _, citation := range citationMap {
		citations = append(citations, citation)
	}
	sort.Slice(citations, func(i, j int) bool {
		return citations[i].PR < citations[j].PR
	})
	return citations
}
