package backtest

import (
	"errors"
	"strings"
	"testing"
)

func TestCompareBaselineJSONAcceptsSummaryHeadlineERR(t *testing.T) {
	window := RecurrenceWindow{Start: "2026-01-01T00:00:00Z", End: "2026-01-31T00:00:00Z"}
	baseline, err := CompareBaselineJSON([]byte(`{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "summary": {
    "headline_err": 0.25
  },
  "window": {
    "start": "2026-01-01T00:00:00Z",
    "end": "2026-01-31T00:00:00Z"
  },
  "metadata": {
    "source_artifact_digest": "sha256:baseline"
  }
}
`), ".relia/baselines/error-recurrence-baseline.json", 0.5, "sha256:baseline", window)

	if err != nil {
		t.Fatalf("CompareBaselineJSON returned error: %v", err)
	}
	if baseline.Status != "current" || baseline.Stale {
		t.Fatalf("baseline = %#v, want current", baseline)
	}
	if baseline.HeadlineERR != 0.25 || baseline.Delta != 0.25 {
		t.Fatalf("baseline values = %#v, want headline 0.25 and delta 0.25", baseline)
	}
}

func TestCompareBaselineJSONMarksDigestAndWindowMismatchStale(t *testing.T) {
	window := RecurrenceWindow{Start: "2026-06-01T00:00:00Z", End: "2026-06-29T00:00:00Z"}
	digestStale, err := CompareBaselineJSON([]byte(`{
  "headline_err": 0.25,
  "window": {
    "start": "2026-06-01T00:00:00Z",
    "end": "2026-06-29T00:00:00Z"
  },
  "metadata": {
    "source_artifact_digest": "sha256:old"
  }
}
`), ".relia/baselines/error-recurrence-baseline.json", 0.5, "sha256:current", window)
	if err != nil {
		t.Fatalf("digest compare returned error: %v", err)
	}
	if digestStale.Status != "stale" || !digestStale.Stale || !strings.Contains(digestStale.Reason, "digest") {
		t.Fatalf("digest stale = %#v, want stale digest reason", digestStale)
	}

	windowStale, err := CompareBaselineJSON([]byte(`{
  "headline_err": 0.25,
  "window": {
    "start": "2026-01-01T00:00:00Z",
    "end": "2026-06-29T00:00:00Z"
  },
  "metadata": {
    "source_artifact_digest": "sha256:current"
  }
}
`), ".relia/baselines/error-recurrence-baseline.json", 0.5, "sha256:current", window)
	if err != nil {
		t.Fatalf("window compare returned error: %v", err)
	}
	if windowStale.Status != "stale" || !windowStale.Stale || !strings.Contains(windowStale.Reason, "window") {
		t.Fatalf("window stale = %#v, want stale window reason", windowStale)
	}
}

func TestCompareBaselineJSONRejectsInvalidPayload(t *testing.T) {
	window := RecurrenceWindow{Start: "2026-01-01T00:00:00Z", End: "2026-01-31T00:00:00Z"}
	_, err := CompareBaselineJSON([]byte(`{`), ".relia/baselines/error-recurrence-baseline.json", 0.5, "sha256:baseline", window)
	if !errors.Is(err, ErrInvalidBaselineJSON) {
		t.Fatalf("invalid JSON error = %v, want ErrInvalidBaselineJSON", err)
	}

	_, err = CompareBaselineJSON([]byte(`{"headline_err": 2}`), ".relia/baselines/error-recurrence-baseline.json", 0.5, "sha256:baseline", window)
	if !errors.Is(err, ErrInvalidBaselineHeadlineERR) {
		t.Fatalf("invalid headline error = %v, want ErrInvalidBaselineHeadlineERR", err)
	}
}

func TestSavedBaselineComparisonUsesRoundedCurrentValues(t *testing.T) {
	baseline := SavedBaselineComparison(".relia/baselines/error-recurrence-baseline.json", 0.3333333)

	if baseline.Status != "saved" || baseline.Stale {
		t.Fatalf("baseline = %#v, want saved and current", baseline)
	}
	if baseline.HeadlineERR != 0.3333 || baseline.Delta != 0 {
		t.Fatalf("baseline values = %#v, want rounded headline and zero delta", baseline)
	}
}
