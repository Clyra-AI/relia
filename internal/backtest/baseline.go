package backtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
)

var (
	ErrInvalidBaselineJSON        = errors.New("invalid baseline JSON")
	ErrInvalidBaselineHeadlineERR = errors.New("invalid baseline headline ERR")
)

func CompareBaselineJSON(content []byte, path string, headlineERR float64, sourceDigest string, window RecurrenceWindow) (BaselineComparison, error) {
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return BaselineComparison{}, ErrInvalidBaselineJSON
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return BaselineComparison{}, ErrInvalidBaselineJSON
	}
	return CompareBaselinePayload(payload, path, headlineERR, sourceDigest, window)
}

func CompareBaselinePayload(payload map[string]any, path string, headlineERR float64, sourceDigest string, window RecurrenceWindow) (BaselineComparison, error) {
	baselineERR, ok := numericValue(payload["headline_err"])
	if !ok {
		if summary, summaryOK := payload["summary"].(map[string]any); summaryOK {
			if summaryERR, headlineOK := numericValue(summary["headline_err"]); headlineOK {
				baselineERR = summaryERR
				ok = true
			}
		}
	}
	if !ok || baselineERR < 0 || baselineERR > 1 {
		return BaselineComparison{}, ErrInvalidBaselineHeadlineERR
	}
	baselineDigest := ""
	if metadata, ok := payload["metadata"].(map[string]any); ok {
		baselineDigest = stringFromAny(metadata["source_artifact_digest"])
	}
	status := "current"
	reason := "Saved baseline was computed from the same source artifact digest."
	stale := false
	if baselineDigest == "" || baselineDigest != sourceDigest {
		status = "stale"
		stale = true
		reason = "Saved baseline source artifact digest differs from the current backtest inputs."
	} else if !baselineWindowMatches(payload, window) {
		status = "stale"
		stale = true
		reason = "Saved baseline window differs from the current backtest window."
	}
	return BaselineComparison{
		Status:      status,
		Path:        path,
		HeadlineERR: roundFloat(baselineERR, 4),
		Delta:       roundFloat(headlineERR-baselineERR, 4),
		Stale:       stale,
		Reason:      reason,
	}, nil
}

func MissingBaselineComparison(path string) BaselineComparison {
	return BaselineComparison{
		Status: "missing",
		Path:   path,
		Stale:  false,
		Reason: "No saved ERR baseline exists yet; use --save-baseline after reviewing the report to create one.",
	}
}

func SavedBaselineComparison(path string, headlineERR float64) BaselineComparison {
	return BaselineComparison{
		Status:      "saved",
		Path:        path,
		HeadlineERR: roundFloat(headlineERR, 4),
		Delta:       0,
		Stale:       false,
		Reason:      "Saved current headline ERR as the comparison baseline.",
	}
}

func baselineWindowMatches(payload map[string]any, window RecurrenceWindow) bool {
	baselineWindow, ok := payload["window"].(map[string]any)
	if !ok {
		return false
	}
	return stringFromAny(baselineWindow["start"]) == window.Start &&
		stringFromAny(baselineWindow["end"]) == window.End
}

func roundFloat(value float64, places int) float64 {
	if places < 0 {
		return value
	}
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
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
