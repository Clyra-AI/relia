package advise

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	assessdoc "github.com/Clyra-AI/relia/internal/assess"
	configdoc "github.com/Clyra-AI/relia/internal/config"
)

type StateErrorKind string

const (
	StateErrorUsage            StateErrorKind = "usage"
	StateErrorArtifactContract StateErrorKind = "artifact_contract"
	StateErrorInternal         StateErrorKind = "internal"
)

type StateError struct {
	Kind    StateErrorKind
	Message string
	Ref     string
	Err     error
}

type ForwardBaseline struct {
	Status         string
	Path           string
	HeadlineERR    float64
	HasHeadlineERR bool
	Reason         string
}

func (e *StateError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	if e.Ref != "" {
		return e.Message + " (" + e.Ref + ")"
	}
	return e.Message
}

func LoadPriorState(root string, statePath string) (PriorState, *StateError) {
	clean, ok := configdoc.CleanRepoPath(statePath)
	if !ok {
		return PriorState{}, &StateError{Kind: StateErrorUsage, Message: "advise --state must be repo-relative"}
	}
	ref := filepath.ToSlash(clean)
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ref)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PriorState{}, nil
		}
		return PriorState{}, &StateError{Kind: StateErrorInternal, Message: "could not read prior advisory state", Err: err}
	}
	var state map[string]any
	if err := json.Unmarshal(content, &state); err != nil {
		return PriorState{}, &StateError{Kind: StateErrorArtifactContract, Message: "prior advisory state is not valid JSON", Ref: ref}
	}
	prior := PriorState{}
	prior.DiffFingerprint, _ = state["diff_fingerprint"].(string)
	prior.SkipReason, _ = state["skip_reason"].(string)
	if metadata, ok := state["metadata"].(map[string]any); ok {
		if generatedAt, _ := metadata["generated_at"].(string); generatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, generatedAt); err == nil {
				prior.GeneratedAt = parsed
			}
		}
		prior.RiskLevel, _ = metadata["risk_level"].(string)
	}
	if assessment, ok := state["assessment"].(map[string]any); ok {
		if riskLevel, _ := assessment["risk_level"].(string); riskLevel != "" && prior.RiskLevel == "" {
			prior.RiskLevel = riskLevel
		}
	}
	return prior, nil
}

func LoadForwardBaseline(root string, baselinePath string) (ForwardBaseline, *StateError) {
	clean, ok := configdoc.CleanRepoPath(baselinePath)
	if !ok {
		return ForwardBaseline{}, &StateError{Kind: StateErrorUsage, Message: "advise --baseline must be repo-relative"}
	}
	ref := filepath.ToSlash(clean)
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ref)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ForwardBaseline{
				Status: "missing",
				Path:   ref,
				Reason: "No saved ERR baseline exists yet; use relia backtest --save-baseline before comparing forward advisory signals.",
			}, nil
		}
		return ForwardBaseline{}, &StateError{Kind: StateErrorInternal, Message: "could not read forward ERR baseline", Err: err}
	}
	var payload map[string]any
	if err := json.Unmarshal(content, &payload); err != nil {
		return ForwardBaseline{}, &StateError{Kind: StateErrorArtifactContract, Message: "forward ERR baseline is not valid JSON", Ref: ref}
	}
	headlineERR, ok := numericValue(payload["headline_err"])
	if !ok {
		if summary, summaryOK := payload["summary"].(map[string]any); summaryOK {
			headlineERR, ok = numericValue(summary["headline_err"])
		}
	}
	if !ok || headlineERR < 0 || headlineERR > 1 {
		return ForwardBaseline{}, &StateError{Kind: StateErrorArtifactContract, Message: "forward ERR baseline must include headline_err between 0 and 1", Ref: ref}
	}
	status, reason := forwardBaselineFreshness(payload)
	return ForwardBaseline{
		Status:         status,
		Path:           ref,
		HeadlineERR:    headlineERR,
		HasHeadlineERR: true,
		Reason:         reason,
	}, nil
}

func forwardBaselineFreshness(payload map[string]any) (string, string) {
	if metadata, ok := payload["metadata"].(map[string]any); ok {
		if digest, _ := metadata["source_artifact_digest"].(string); strings.TrimSpace(digest) != "" {
			if window, windowOK := payload["window"].(map[string]any); windowOK {
				start, _ := window["start"].(string)
				end, _ := window["end"].(string)
				if strings.TrimSpace(start) != "" && strings.TrimSpace(end) != "" {
					return "current", "Loaded saved ERR baseline for forward advisory signal tracking."
				}
			}
			return "stale", "Saved ERR baseline is missing baseline window metadata; regenerate with relia backtest --save-baseline before comparing forward advisory signals."
		}
	}
	return "stale", "Saved ERR baseline is missing source artifact digest metadata; regenerate with relia backtest --save-baseline before comparing forward advisory signals."
}

func BuildForwardSignal(
	schemaVersion string,
	inputPath string,
	assessment assessdoc.RiskAssessment,
	settings configdoc.AdviseSettings,
	diffFingerprint string,
	baseline ForwardBaseline,
	shouldComment bool,
	skipReason string,
	generatedAt time.Time,
) map[string]any {
	if baseline.Status == "" {
		baseline = ForwardBaseline{
			Status: "missing",
			Path:   ".relia/baselines/error-recurrence-baseline.json",
			Reason: "No saved ERR baseline exists yet; use relia backtest --save-baseline before comparing forward advisory signals.",
		}
	}
	baselineData := map[string]any{
		"status": baseline.Status,
		"path":   baseline.Path,
		"reason": baseline.Reason,
	}
	if baseline.HasHeadlineERR {
		baselineData["headline_err"] = baseline.HeadlineERR
	}
	coverage, _ := assessment.Metadata["coverage"].(string)
	if coverage == "" {
		coverage = coverageFromRiskLevel(assessment.RiskLevel)
	}
	commentAction := "skip"
	if shouldComment {
		commentAction = "publish"
	}
	return map[string]any{
		"object_type":      "relia.forward_signal",
		"schema_version":   schemaVersion,
		"generated_at":     generatedAt.UTC().Format(time.RFC3339),
		"input_path":       inputPath,
		"diff_fingerprint": diffFingerprint,
		"assessment_id":    assessment.AssessmentID,
		"risk_level":       PublishedRiskLevel(assessment, skipReason),
		"coverage":         coverage,
		"comment_action":   commentAction,
		"skip_reason":      skipReason,
		"baseline":         baselineData,
		"metadata": map[string]any{
			"advisory_only":                 true,
			"gate_enabled_default":          false,
			"tracks_forward_err":            true,
			"forward_err_baseline_required": false,
			"max_comments_per_pr":           settings.MaxCommentsPerPR,
			"update_in_place":               settings.UpdateInPlace,
			"reassess_debounce_minutes":     settings.ReassessDebounceMinutes,
			"min_confidence":                settings.MinConfidence,
		},
	}
}

func StateDocument(
	schemaVersion string,
	inputPath string,
	assessment assessdoc.RiskAssessment,
	settings configdoc.AdviseSettings,
	diffFingerprint string,
	previousState PriorState,
	shouldComment bool,
	skipReason string,
	generatedAt time.Time,
	forwardSignals ...map[string]any,
) map[string]any {
	generatedAtValue := generatedAt.UTC().Format(time.RFC3339)
	stateDiffFingerprint := diffFingerprint
	stateMetadata := map[string]any{
		"generated_by":              "relia advise",
		"generated_at":              generatedAtValue,
		"hosted_service_required":   false,
		"github_api_required_later": shouldComment,
		"risk_level":                PublishedRiskLevel(assessment, skipReason),
	}
	if skipReason == "reassess_debounce_window" && previousState.DiffFingerprint != "" {
		stateDiffFingerprint = previousState.DiffFingerprint
		if !previousState.GeneratedAt.IsZero() {
			stateMetadata["generated_at"] = previousState.GeneratedAt.UTC().Format(time.RFC3339)
		}
		stateMetadata["debounced_diff_fingerprint"] = diffFingerprint
		stateMetadata["debounced_at"] = generatedAtValue
	}
	forwardSignal := BuildForwardSignal(schemaVersion, inputPath, assessment, settings, diffFingerprint, ForwardBaseline{}, shouldComment, skipReason, generatedAt)
	if len(forwardSignals) > 0 && forwardSignals[0] != nil {
		forwardSignal = forwardSignals[0]
	}
	return map[string]any{
		"object_type":               "relia.advisory_state",
		"schema_version":            schemaVersion,
		"input_path":                inputPath,
		"diff_fingerprint":          stateDiffFingerprint,
		"previous_diff_fingerprint": previousState.DiffFingerprint,
		"should_comment":            shouldComment,
		"skip_reason":               skipReason,
		"assessment":                assessment,
		"comment_strategy":          CommentStrategy(settings),
		"comment_marker":            commentMarker,
		"forward_signal":            forwardSignal,
		"metadata":                  stateMetadata,
	}
}

func coverageFromRiskLevel(riskLevel string) string {
	switch riskLevel {
	case "match_high", "match_medium":
		return "covered_risky"
	case "covered_clean":
		return "covered_clean"
	default:
		return "no_coverage"
	}
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
