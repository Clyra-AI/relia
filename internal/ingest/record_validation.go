package ingest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func ValidateEventMemorySource(event map[string]any, ref string) *Error {
	for _, path := range []string{
		"object_type",
		"event_type",
		"event_kind",
		"type",
		"kind",
		"source",
		"source_kind",
		"source.object_type",
		"source.type",
		"source.kind",
		"source.memory_source",
		"memory_source",
		"metadata.source",
		"metadata.source_kind",
		"metadata.source_type",
		"metadata.object_type",
		"metadata.event_type",
		"metadata.event_kind",
		"metadata.type",
		"metadata.kind",
		"metadata.source.object_type",
		"metadata.source.type",
		"metadata.source.kind",
		"metadata.source.memory_source",
		"metadata.memory_source",
	} {
		if unverifiedMemorySourceKind(stringField(event, path)) {
			return unverifiedMemorySourceError(ref)
		}
	}
	return nil
}

func NormalizeRepo(event map[string]any, ref string) (Repo, *Error) {
	repo := Repo{Provider: "github"}
	if value, ok := nestedField(event, "repo"); ok {
		switch typed := value.(type) {
		case map[string]any:
			if provider := stringFromAny(typed["provider"]); provider != "" {
				repo.Provider = provider
			}
			repo.Owner = stringFromAny(typed["owner"])
			repo.Name = stringFromAny(typed["name"])
		case string:
			owner, name, ok := strings.Cut(typed, "/")
			if ok {
				repo.Owner = strings.TrimSpace(owner)
				repo.Name = strings.TrimSpace(name)
			}
		}
	}
	if repo.Owner == "" {
		repo.Owner = stringField(event, "repo_owner")
	}
	if repo.Name == "" {
		repo.Name = stringField(event, "repo_name")
	}
	if repo.Provider != "github" {
		return repo, artifactContractError("experience repo.provider must be github", ref)
	}
	if repo.Owner == "" || repo.Name == "" {
		return repo, artifactContractError("experience repo must include owner and name", ref)
	}
	return repo, nil
}

func NormalizeAction(event map[string]any, ref string) (Action, *Error) {
	pr, commandErr := requiredPositiveIntField(event, ref, "experience record PR number", "pr", "action.pr")
	if commandErr != nil {
		return Action{}, commandErr
	}
	action := Action{
		PR:     pr,
		Commit: stringField(event, "commit", "action.commit"),
	}
	if action.Commit == "" {
		commits := stringListField(event, "commits", "action.commits")
		if len(commits) > 0 {
			action.Commit = commits[0]
		}
	}
	if action.Commit == "" {
		return action, artifactContractError("experience record must include commit", ref)
	}
	return action, nil
}

func NormalizeProvenance(event map[string]any, ref string) (Provenance, *Error) {
	urls := stringListField(event, "provenance_urls", "provenance.urls")
	for _, key := range []string{"pr_url", "check_run_url", "revert_url", "review_url"} {
		if value := stringField(event, key, "provenance."+key); value != "" {
			urls = append(urls, value)
		}
	}
	urls = uniqueStrings(urls)
	if len(urls) == 0 {
		return Provenance{}, provenanceIntegrityError("experience record must include at least one provenance URL", ref)
	}
	for _, value := range urls {
		if !ValidGitHubProvenanceURLShape(value) {
			return Provenance{}, provenanceIntegrityError("experience provenance URL must be a canonical https://github.com/ URL", ref)
		}
	}
	return Provenance{URLs: urls}, nil
}

func NormalizeContext(event map[string]any, action Action, ref string) (Context, *Error) {
	paths := stringListField(event, "paths", "context.paths")
	if len(paths) == 0 {
		return Context{}, artifactContractError("experience record must include at least one context path", ref)
	}
	context := Context{
		Paths:           paths,
		DiffFingerprint: stringField(event, "diff_fingerprint", "context.diff_fingerprint"),
	}
	if context.DiffFingerprint == "" {
		context.DiffFingerprint = sha256String(fmt.Sprintf("%d|%s|%s", action.PR, action.Commit, strings.Join(paths, "|")))
	}
	return context, nil
}

func CanonicalDistillInputRecord(event map[string]any, ref string) (Record, bool, *Error) {
	if stringField(event, "object_type") != "relia.experience_record" {
		return Record{}, false, nil
	}
	if commandErr := ValidateEventMemorySource(event, ref); commandErr != nil {
		return Record{}, true, commandErr
	}
	if commandErr := validateCanonicalDistillInputCompleteness(event, ref); commandErr != nil {
		return Record{}, true, commandErr
	}
	content, err := json.Marshal(event)
	if err != nil {
		return Record{}, true, internalError("could not decode canonical distill input experience record", err)
	}
	var record Record
	if err := DecodeJSONUseNumber(string(content), &record); err != nil {
		return Record{}, true, artifactContractError("canonical distill input experience record is invalid", ref)
	}
	return record, true, nil
}

func ValidateRecord(record Record, ref string, schemaVersion string) (time.Time, *Error) {
	if record.ObjectType != "relia.experience_record" {
		return time.Time{}, artifactContractError("backtest experience object_type must be relia.experience_record", ref)
	}
	if record.SchemaVersion != schemaVersion {
		return time.Time{}, artifactContractError("backtest experience schema_version must be "+schemaVersion, ref)
	}
	if record.ShareScope != "private" {
		return time.Time{}, redactionSafetyError("backtest experience share_scope must be private", ref)
	}
	if record.RedactionStatus != "applied" {
		return time.Time{}, redactionSafetyError("backtest experience redaction_status must be applied", ref)
	}
	if record.OrgEligible {
		return time.Time{}, artifactContractError("backtest experience org_eligible must be false", ref)
	}
	if strings.TrimSpace(record.ExperienceID) == "" {
		return time.Time{}, artifactContractError("backtest experience missing experience_id", ref)
	}
	recordedAt, err := time.Parse(time.RFC3339, record.RecordedAt)
	if err != nil {
		return time.Time{}, artifactContractError("backtest experience recorded_at must be RFC3339", ref)
	}
	if record.Repo.Provider != "github" || record.Repo.Owner == "" || record.Repo.Name == "" {
		return time.Time{}, artifactContractError("backtest experience repo must include github owner and name", ref)
	}
	if record.Action.PR < 1 {
		return time.Time{}, provenanceIntegrityError("backtest experience action.pr must be a positive integer", ref)
	}
	if strings.TrimSpace(record.Action.Commit) == "" {
		return time.Time{}, provenanceIntegrityError("backtest experience action.commit must be provided", ref)
	}
	if !validOutcomeKind(record.Outcome.Kind) || !validTerminalState(record.Outcome.TerminalState) {
		return time.Time{}, artifactContractError("backtest experience outcome is invalid", ref)
	}
	if strings.TrimSpace(record.Outcome.Signature.SignatureID) == "" {
		return time.Time{}, artifactContractError("backtest experience outcome.signature.signature_id must be provided", ref)
	}
	if !validExtractionConfidence(record.Outcome.Signature.ExtractionConfidence) {
		return time.Time{}, artifactContractError("backtest experience signature extraction_confidence is invalid", ref)
	}
	switch record.Attribution.ActorKind {
	case "agent", "human", "uncertain":
	default:
		return time.Time{}, artifactContractError("backtest experience attribution actor_kind must be agent, human, or uncertain", ref)
	}
	switch record.Attribution.Method {
	case "bot_login", "coauthor_trailer", "pr_label", "manual", "uncertain":
	default:
		return time.Time{}, artifactContractError("backtest experience attribution method is invalid", ref)
	}
	if record.Attribution.Confidence < 0 || record.Attribution.Confidence > 1 || math.IsNaN(record.Attribution.Confidence) || math.IsInf(record.Attribution.Confidence, 0) {
		return time.Time{}, artifactContractError("backtest experience attribution confidence must be between 0 and 1", ref)
	}
	if len(record.Context.Paths) == 0 {
		return time.Time{}, artifactContractError("backtest experience context.paths must include at least one path", ref)
	}
	if strings.TrimSpace(record.Context.DiffFingerprint) == "" {
		return time.Time{}, artifactContractError("backtest experience context.diff_fingerprint must be provided", ref)
	}
	for _, path := range record.Context.Paths {
		if _, ok := cleanRepoPath(path); !ok {
			return time.Time{}, artifactContractError("backtest experience context.paths entries must be repo-relative", ref)
		}
	}
	if record.FlakeDiscount < 0 || record.FlakeDiscount > 1 || math.IsNaN(record.FlakeDiscount) || math.IsInf(record.FlakeDiscount, 0) {
		return time.Time{}, artifactContractError("backtest experience flake_discount must be between 0 and 1", ref)
	}
	if len(record.Provenance.URLs) == 0 {
		return time.Time{}, provenanceIntegrityError("backtest experience must include provenance URLs", ref)
	}
	for _, value := range record.Provenance.URLs {
		if !ValidGitHubProvenanceURLShape(value) {
			return time.Time{}, provenanceIntegrityError("backtest experience provenance URL must be a canonical https://github.com/ URL", ref)
		}
		if !GitHubProvenanceURLRepoMatchesRecord(value, record) {
			return time.Time{}, provenanceIntegrityError("backtest experience provenance URL repo must match experience repo", ref)
		}
		if number, ok := GitHubPullRequestURLPathNumber(value); ok && number != record.Action.PR {
			return time.Time{}, provenanceIntegrityError("backtest experience pull request provenance URL must match action.pr", ref)
		}
	}
	if commandErr := validateRecordMemorySource(record, ref); commandErr != nil {
		return time.Time{}, commandErr
	}
	return recordedAt.UTC(), nil
}

func validateCanonicalDistillInputCompleteness(event map[string]any, ref string) *Error {
	for _, path := range []string{"action.commit", "attribution.method", "context.diff_fingerprint"} {
		if stringField(event, path) == "" {
			return artifactContractError("canonical distill input "+path+" must be provided", ref)
		}
	}
	confidence, ok := nestedField(event, "attribution.confidence")
	if !ok {
		return artifactContractError("canonical distill input attribution.confidence must be provided", ref)
	}
	if _, valid := numericValue(confidence); !valid {
		return artifactContractError("canonical distill input attribution.confidence must be numeric", ref)
	}
	return nil
}

func validateRecordMemorySource(record Record, ref string) *Error {
	for _, path := range []string{
		"source",
		"source_kind",
		"source_type",
		"memory_source",
		"object_type",
		"event_type",
		"event_kind",
		"type",
		"kind",
		"source.object_type",
		"source.kind",
		"source.type",
		"source.memory_source",
	} {
		if unverifiedMemorySourceKind(metadataStringField(record.Metadata, path)) {
			return unverifiedMemorySourceError(ref)
		}
	}
	return nil
}

func metadataStringField(metadata map[string]any, paths ...string) string {
	if metadata == nil {
		return ""
	}
	for _, path := range paths {
		if value, ok := nestedField(metadata, path); ok {
			if converted := stringFromAny(value); converted != "" {
				return converted
			}
		}
	}
	return ""
}

func unverifiedMemorySourceKind(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	compact := strings.ReplaceAll(normalized, "_", "")
	switch normalized {
	case "agent_self_report", "self_report", "self_reported", "agent_reflection", "reflection", "agent_observation", "agent_note":
		return true
	default:
		switch compact {
		case "agentselfreport", "selfreport", "selfreported", "agentreflection", "reflection", "agentobservation", "agentnote":
			return true
		default:
			return strings.Contains(normalized, "self_report") ||
				strings.Contains(normalized, "reflection") ||
				strings.Contains(compact, "selfreport") ||
				strings.Contains(compact, "reflection")
		}
	}
}

func unverifiedMemorySourceError(ref string) *Error {
	return artifactContractError("agent self-reports and reflections cannot become Relia experience records or memory sources in the MVP", ref)
}

func nestedField(event map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = event
	for _, part := range parts {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapping[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func stringField(event map[string]any, paths ...string) string {
	for _, path := range paths {
		if value, ok := nestedField(event, path); ok {
			if converted := stringFromAny(value); converted != "" {
				return converted
			}
		}
	}
	return ""
}

func requiredPositiveIntField(event map[string]any, ref string, fieldDescription string, paths ...string) (int, *Error) {
	for _, path := range paths {
		value, ok := nestedField(event, path)
		if !ok {
			continue
		}
		converted, ok := intFromAny(value)
		if !ok || converted < 1 {
			return 0, provenanceIntegrityError(fieldDescription+" must be a positive integer", ref)
		}
		return converted, nil
	}
	return 0, provenanceIntegrityError(fieldDescription+" must be provided", ref)
}

func intFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) {
			return 0, false
		}
		return int64ToInt(int64(typed))
	case int:
		return typed, true
	case json.Number:
		converted, err := typed.Int64()
		if err == nil {
			return int64ToInt(converted)
		}
	case string:
		converted, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return converted, true
		}
	}
	return 0, false
}

func int64ToInt(value int64) (int, bool) {
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	if value < minInt || value > maxInt {
		return 0, false
	}
	return int(value), true
}

func stringListField(event map[string]any, paths ...string) []string {
	for _, path := range paths {
		if value, ok := nestedField(event, path); ok {
			switch typed := value.(type) {
			case []any:
				var result []string
				for _, item := range typed {
					if converted := stringFromAny(item); converted != "" {
						result = append(result, converted)
					}
				}
				if len(result) > 0 {
					return result
				}
			case []string:
				if len(typed) > 0 {
					return typed
				}
			case string:
				if strings.TrimSpace(typed) != "" {
					return []string{strings.TrimSpace(typed)}
				}
			}
		}
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func sha256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + fmt.Sprintf("%x", digest)
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return typed, true
	case int:
		return float64(typed), true
	case json.Number:
		converted, err := typed.Float64()
		if err == nil && !math.IsNaN(converted) && !math.IsInf(converted, 0) {
			return converted, true
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, false
		}
		converted, err := strconv.ParseFloat(trimmed, 64)
		if err == nil && !math.IsNaN(converted) && !math.IsInf(converted, 0) {
			return converted, true
		}
	}
	return 0, false
}

func validOutcomeKind(kind string) bool {
	switch kind {
	case "merged_clean", "ci_failure", "revert", "review_correction", "fix_held":
		return true
	default:
		return false
	}
}

func validTerminalState(value string) bool {
	switch value {
	case "passed", "failed", "reverted", "corrected", "held":
		return true
	default:
		return false
	}
}

func validExtractionConfidence(value string) bool {
	switch value {
	case "structured", "log_parsed_high", "log_parsed_low", "unknown":
		return true
	default:
		return false
	}
}

func cleanRepoPath(rel string) (string, bool) {
	trimmed := strings.TrimSpace(rel)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return "", false
	}
	clean := filepath.Clean(trimmed)
	cleanSlash := filepath.ToSlash(clean)
	if clean == "." || clean == ".." || strings.HasPrefix(cleanSlash, "../") {
		return "", false
	}
	for _, part := range strings.Split(cleanSlash, "/") {
		if part == ".." {
			return "", false
		}
	}
	return clean, true
}
