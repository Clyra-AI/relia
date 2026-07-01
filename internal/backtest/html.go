package backtest

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

func RenderHTML(report RecurrenceReport) string {
	var builder strings.Builder
	builder.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>Relia Backtest ")
	builder.WriteString(html.EscapeString(report.ReportID))
	builder.WriteString("</title></head><body>\n")
	builder.WriteString("<h1>Relia Backtest</h1>\n")
	builder.WriteString(fmt.Sprintf("<p>Headline ERR: <strong>%s</strong> (%d confirmed / %d agent failures)</p>\n",
		html.EscapeString(report.Summary.HeadlineERRPercent),
		report.Summary.ConfirmedRecurrenceCount,
		report.Summary.AgentFailureDenominator))
	builder.WriteString("<p>")
	builder.WriteString(html.EscapeString(report.OperatorFeedback.Summary))
	builder.WriteString("</p>\n")
	builder.WriteString(fmt.Sprintf("<p>PRs analyzed: %d (agent-attributed: %d)</p>\n",
		report.Metrics.PRsAnalyzed,
		report.Metrics.AgentAttributedPRs))
	builder.WriteString("<p>Badge: ")
	builder.WriteString(html.EscapeString(report.Badge.Label + " " + report.Badge.Message))
	builder.WriteString("</p>\n")
	builder.WriteString("<h2>Top Repeated Mistakes</h2>\n<ol>\n")
	for _, mistake := range report.TopRepeatedMistakes {
		builder.WriteString("<li>")
		builder.WriteString(html.EscapeString(fmt.Sprintf("%s repeated %dx across PRs %s", mistake.SignatureID, mistake.RepeatCount, joinInts(mistake.PRs, ", "))))
		builder.WriteString("</li>\n")
	}
	builder.WriteString("</ol>\n")
	builder.WriteString("<h2>Confirmed Recurrences</h2>\n<ul>\n")
	for _, pair := range report.ConfirmedRecurrences {
		builder.WriteString("<li>")
		builder.WriteString(html.EscapeString(fmt.Sprintf("PR #%d repeats PR #%d (%s)", pair.CurrentPR, pair.PriorPR, pair.SignatureID)))
		if pair.CurrentURL != "" {
			builder.WriteString(" <a href=\"")
			builder.WriteString(html.EscapeString(pair.CurrentURL))
			builder.WriteString("\">current</a>")
		}
		if pair.PriorURL != "" {
			builder.WriteString(" <a href=\"")
			builder.WriteString(html.EscapeString(pair.PriorURL))
			builder.WriteString("\">prior</a>")
		}
		builder.WriteString("</li>\n")
	}
	builder.WriteString("</ul>\n<h2>Possible Recurrences</h2>\n<ul>\n")
	for _, pair := range report.PossibleRecurrences {
		builder.WriteString("<li>")
		builder.WriteString(html.EscapeString(fmt.Sprintf("PR #%d possibly repeats PR #%d (%s); excluded from headline ERR", pair.CurrentPR, pair.PriorPR, pair.SignatureID)))
		builder.WriteString("</li>\n")
	}
	builder.WriteString("</ul>\n<h2>Flake Discounts</h2>\n<ul>\n")
	for _, flake := range report.FlakeDiscounts {
		builder.WriteString("<li>")
		builder.WriteString(html.EscapeString(fmt.Sprintf("PR #%d %s: %s", flake.PR, flake.SignatureID, flake.Reason)))
		builder.WriteString("</li>\n")
	}
	builder.WriteString("</ul>\n<h2>Diagnostics</h2>\n<ul>\n")
	for _, diagnostic := range report.Diagnostics {
		builder.WriteString("<li>")
		builder.WriteString(html.EscapeString(fmt.Sprintf("%s: %s (%s)", diagnostic.Status, diagnostic.Message, diagnostic.Ref)))
		builder.WriteString("</li>\n")
	}
	builder.WriteString("</ul>\n</body></html>\n")
	return builder.String()
}

func joinInts(values []int, sep string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, sep)
}
