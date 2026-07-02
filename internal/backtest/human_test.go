package backtest

import (
	"strings"
	"testing"
)

func TestWriteHumanDetailsRendersSummaryAndDiagnostics(t *testing.T) {
	report := RecurrenceReport{
		Summary: RecurrenceSummary{
			AgentFailureDenominator:  3,
			ConfirmedRecurrenceCount: 1,
			PossibleRecurrenceCount:  2,
			HeadlineERRPercent:       "33.3%",
		},
		Metrics: RecurrenceMetrics{
			PRsAnalyzed:                5,
			AgentAttributedPRs:         4,
			AgentFailuresByOutcomeKind: map[string]int{"review_correction": 1, "ci_failure": 2},
		},
		TopRepeatedMistakes: []TopRepeatedMistake{
			{Rank: 1, SignatureID: "sig_timeout", RepeatCount: 2, PRs: []int{7, 9}},
		},
		Badge: ReportBadge{Label: "Relia", Message: "ERR 33.3%"},
		Diagnostics: []ReportDiagnostic{
			{Status: "warn", Message: "Saved baseline is stale.", Ref: ".relia/baselines/err.json"},
		},
	}

	var output strings.Builder
	if err := WriteHumanDetails(&output, report, ".relia/reports/backtest.html"); err != nil {
		t.Fatalf("WriteHumanDetails returned error: %v", err)
	}

	for _, want := range []string{
		"PRs analyzed: 5 (agent-attributed: 4)",
		"Agent failures: 3 (ci_failure: 2, review_correction: 1)",
		"Confirmed recurrences: 1 (33.3% of agent failures)",
		"Possible recurrences: 2 (excluded from headline)",
		"1. sig_timeout 2x (PR #7, #9)",
		"Error recurrence rate: 33.3%",
		"Report: .relia/reports/backtest.html",
		"Badge: Relia ERR 33.3%",
		"- warn: Saved baseline is stale. (.relia/baselines/err.json)",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("human details missing %q:\n%s", want, output.String())
		}
	}
}

func TestWriteHumanDetailsHandlesEmptyOptionalSections(t *testing.T) {
	report := RecurrenceReport{
		Summary: RecurrenceSummary{
			HeadlineERRPercent: "0.0%",
		},
		Badge: ReportBadge{Label: "Relia", Message: "ERR 0.0%"},
	}

	var output strings.Builder
	if err := WriteHumanDetails(&output, report, ""); err != nil {
		t.Fatalf("WriteHumanDetails returned error: %v", err)
	}

	rendered := output.String()
	for _, want := range []string{
		"Agent failures: 0 (none)",
		"Top repeated mistakes:\n   none",
		"Badge: Relia ERR 0.0%",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("human details missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Report:") || strings.Contains(rendered, "Diagnostics:") {
		t.Fatalf("human details rendered optional sections unexpectedly:\n%s", rendered)
	}
}
