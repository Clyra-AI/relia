package advise

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

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
