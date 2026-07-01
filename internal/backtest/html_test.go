package backtest

import (
	"strings"
	"testing"
)

func TestRenderHTMLEscapesReportFieldsAndFormatsHeadline(t *testing.T) {
	html := RenderHTML(RecurrenceReport{
		ReportID: "backtest_<unsafe>",
		Summary: RecurrenceSummary{
			HeadlineERRPercent:       "16.7%",
			ConfirmedRecurrenceCount: 2,
			AgentFailureDenominator:  12,
		},
		Metrics: RecurrenceMetrics{
			PRsAnalyzed:        20,
			AgentAttributedPRs: 18,
		},
		OperatorFeedback: OperatorFeedback{
			Summary: "Review <confirmed> recurrences.",
		},
		Badge: ReportBadge{
			Label:   "Relia",
			Message: "ERR 16.7%",
		},
		TopRepeatedMistakes: []TopRepeatedMistake{
			{SignatureID: "sig_<x>", RepeatCount: 1, PRs: []int{7, 8}},
		},
		ConfirmedRecurrences: []RecurrencePair{
			{
				CurrentPR:   8,
				PriorPR:     7,
				SignatureID: "sig_<x>",
				CurrentURL:  "https://example.test/current?x=<y>",
				PriorURL:    "https://example.test/prior?x=<y>",
			},
		},
		Diagnostics: []ReportDiagnostic{
			{Status: "pass", Message: "Safe <message>", Ref: "schemas/recurrence-report.schema.json"},
		},
	})

	for _, want := range []string{
		"Relia Backtest backtest_&lt;unsafe&gt;",
		"Headline ERR: <strong>16.7%</strong> (2 confirmed / 12 agent failures)",
		"Review &lt;confirmed&gt; recurrences.",
		"sig_&lt;x&gt; repeated 1x across PRs 7, 8",
		"https://example.test/current?x=&lt;y&gt;",
		"pass: Safe &lt;message&gt; (schemas/recurrence-report.schema.json)",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered HTML missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "%!d") || strings.Contains(html, "%!(EXTRA") {
		t.Fatalf("rendered HTML contains fmt artifact:\n%s", html)
	}
}
