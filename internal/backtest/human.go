package backtest

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

func WriteHumanDetails(writer io.Writer, report RecurrenceReport, reportPath string) error {
	if _, err := fmt.Fprintf(writer, "  PRs analyzed: %d (agent-attributed: %d)\n", report.Metrics.PRsAnalyzed, report.Metrics.AgentAttributedPRs); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  Agent failures: %d (%s)\n",
		report.Summary.AgentFailureDenominator,
		formatOutcomeKindCounts(report.Metrics.AgentFailuresByOutcomeKind)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  Confirmed recurrences: %d (%s of agent failures)\n",
		report.Summary.ConfirmedRecurrenceCount,
		report.Summary.HeadlineERRPercent); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  Possible recurrences: %d (excluded from headline)\n", report.Summary.PossibleRecurrenceCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer, "  Top repeated mistakes:"); err != nil {
		return err
	}
	if len(report.TopRepeatedMistakes) == 0 {
		if _, err := fmt.Fprintln(writer, "   none"); err != nil {
			return err
		}
	} else {
		for _, mistake := range report.TopRepeatedMistakes {
			if _, err := fmt.Fprintf(writer, "   %d. %s %dx (PR #%s)\n",
				mistake.Rank,
				mistake.SignatureID,
				mistake.RepeatCount,
				joinInts(mistake.PRs, ", #")); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintf(writer, "  Error recurrence rate: %s\n", report.Summary.HeadlineERRPercent); err != nil {
		return err
	}
	if reportPath != "" {
		if _, err := fmt.Fprintf(writer, "  Report: %s\n", reportPath); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(writer, "  Badge: %s %s\n", report.Badge.Label, report.Badge.Message); err != nil {
		return err
	}
	if len(report.Diagnostics) > 0 {
		if _, err := fmt.Fprintln(writer, "  Diagnostics:"); err != nil {
			return err
		}
		for _, diagnostic := range report.Diagnostics {
			if _, err := fmt.Fprintf(writer, "   - %s: %s (%s)\n", diagnostic.Status, diagnostic.Message, diagnostic.Ref); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatOutcomeKindCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s: %d", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}
