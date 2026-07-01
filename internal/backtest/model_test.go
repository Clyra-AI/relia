package backtest

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBaselineComparisonMissingOmitsERRAndDelta(t *testing.T) {
	encoded, err := json.Marshal(BaselineComparison{
		Status: "missing",
		Path:   ".relia/baselines/error-recurrence-baseline.json",
		Reason: "No saved ERR baseline exists yet.",
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	text := string(encoded)
	if strings.Contains(text, "headline_err") {
		t.Fatalf("missing baseline encoded headline_err: %s", text)
	}
	if strings.Contains(text, "delta") {
		t.Fatalf("missing baseline encoded delta: %s", text)
	}
}

func TestBaselineComparisonCurrentIncludesERRAndDelta(t *testing.T) {
	encoded, err := json.Marshal(BaselineComparison{
		Status:      "current",
		Path:        ".relia/baselines/error-recurrence-baseline.json",
		HeadlineERR: 0.25,
		Delta:       -0.05,
		Reason:      "Saved baseline was computed from the same source artifact digest.",
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"headline_err":0.25`) {
		t.Fatalf("current baseline omitted headline_err: %s", text)
	}
	if !strings.Contains(text, `"delta":-0.05`) {
		t.Fatalf("current baseline omitted delta: %s", text)
	}
}
