package main

import (
	"fmt"
	"os"
	"time"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
	configdoc "github.com/Clyra-AI/relia/internal/config"
)

func backtestResult(args []string, start time.Time) CommandResult {
	options, parseErr := backtestdoc.ParseArgs(args)
	withFormat := func(result CommandResult) CommandResult {
		if options.FormatExplicit && options.Format == "json" {
			result.MachineReadable = true
		}
		return result
	}
	if parseErr != nil {
		return withFormat(errorResult("backtest", "backtest", usageError(parseErr.Message), start))
	}
	wd, err := os.Getwd()
	if err != nil {
		return withFormat(errorResult("backtest", "backtest", internalError("could not inspect working directory", err), start))
	}
	root, ok := configdoc.FindRepoRoot(wd)
	if !ok {
		return withFormat(errorResult("backtest", "backtest", configError("could not locate repository root from current directory"), start))
	}
	warnings, commandErr := validateReliaConfig(root)
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}
	config, commandErr := readReliaConfig(root)
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}
	records, sourceArtifacts, sourceDigest, commandErr := loadBacktestExperiences(root)
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}
	windowDays, _ := backtestdoc.ParseWindowDays(options.Window)
	report, commandErr := buildRecurrenceReport(root, config, records, sourceArtifacts, sourceDigest, options, windowDays)
	if commandErr != nil {
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}
	var baselineSnapshot backtestBaselineSnapshot
	baselineSaved := false
	if options.SaveBaseline && report.Gate.Status != "fail" {
		baselineSnapshot, commandErr = snapshotBacktestBaseline(root, options.BaselinePath)
		if commandErr != nil {
			return withFormat(errorResult("backtest", "backtest", commandErr, start))
		}
		if commandErr := saveBacktestBaseline(root, report, options.BaselinePath); commandErr != nil {
			if rollbackErr := restoreBacktestBaseline(baselineSnapshot); rollbackErr != nil {
				commandErr.Message = commandErr.Message + "; additionally could not roll back ERR baseline: " + rollbackErr.Message
			}
			return withFormat(errorResult("backtest", "backtest", commandErr, start))
		}
		baselineSaved = true
		report.Baseline, commandErr = savedBacktestBaselineComparison(options.BaselinePath, report.HeadlineERR)
		if commandErr != nil {
			return withFormat(errorResult("backtest", "backtest", commandErr, start))
		}
		report.Diagnostics = backtestdoc.BuildReportDiagnostics(report.Summary, report.Baseline, report.SourceArtifacts)
		report.Badge = backtestdoc.BuildReportBadge(report)
	}
	jsonReportPath, htmlReportPath, commandErr := writeBacktestReports(root, report, options.ReportDir)
	if commandErr != nil {
		if baselineSaved {
			if rollbackErr := restoreBacktestBaseline(baselineSnapshot); rollbackErr != nil {
				commandErr.Message = commandErr.Message + "; additionally could not roll back ERR baseline: " + rollbackErr.Message
			}
		}
		return withFormat(errorResult("backtest", "backtest", commandErr, start))
	}

	result := passResult("backtest", "backtest", "computed conservative recurrence backtest", start, map[string]any{
		"window":                         report.Window,
		"repo_id":                        report.Metadata["repo_id"],
		"experiences_total":              report.Summary.ExperienceCount,
		"experiences_agent_attributed":   report.Metrics.AgentAttributedExperiences,
		"agent_attributed_prs":           report.Metrics.AgentAttributedPRs,
		"agent_failures_by_outcome_kind": report.Metrics.AgentFailuresByOutcomeKind,
		"headline_err":                   report.HeadlineERR,
		"headline_err_percent":           report.Summary.HeadlineERRPercent,
		"error_recurrence_rate":          report.HeadlineERR,
		"recurrences_confirmed":          report.Summary.ConfirmedRecurrenceCount,
		"recurrences_possible":           report.Summary.PossibleRecurrenceCount,
		"confirmed_recurrences":          report.Summary.ConfirmedRecurrenceCount,
		"possible_recurrences":           report.Summary.PossibleRecurrenceCount,
		"flake_discounted_count":         report.Summary.FlakeDiscountedCount,
		"attribution_uncertain_count":    report.Summary.AttributionUncertainCount,
		"top_repeated_mistakes":          report.TopRepeatedMistakes,
		"diagnostics":                    report.Diagnostics,
		"operator_feedback":              report.OperatorFeedback,
		"badge":                          report.Badge,
		"baseline":                       report.Baseline,
		"baseline_ref":                   report.Baseline.Path,
		"gate":                           report.Gate,
		"report":                         report,
		"report_path":                    htmlReportPath,
		"json_report_path":               jsonReportPath,
		"html_report_path":               htmlReportPath,
	})
	result.Warnings = append(result.Warnings, warnings...)
	result.EvidenceRefs = append(result.EvidenceRefs,
		"schemas/recurrence-report.schema.json",
		"docs/product/prd.md#ingest-attribute-backtest-report",
	)
	for _, artifact := range sourceArtifacts {
		result.EvidenceRefs = append(result.EvidenceRefs, artifact)
		result.Artifacts = append(result.Artifacts, ArtifactRef{Kind: "experience_shard", Path: artifact})
	}
	result.Artifacts = append(result.Artifacts,
		ArtifactRef{Kind: "recurrence_report_json", Path: jsonReportPath},
		ArtifactRef{Kind: "recurrence_report_html", Path: htmlReportPath},
	)
	result.RedactionStatus = "applied"
	if report.Gate.Status == "fail" {
		result.Status = "error"
		result.ExitCode = ExitGate
		result.Errors = append(result.Errors, CommandError{
			Type:        "recurrence_gate_failed",
			Message:     fmt.Sprintf("headline ERR %.4f exceeds configured gate %.4f", report.HeadlineERR, backtestdoc.GateThresholdValue(report.Gate)),
			ExitCode:    ExitGate,
			Remediation: "Leave gate.enabled false for advisory-only MVP behavior or raise the explicit threshold after reviewing the recurrence report.",
			Ref:         report.Gate.Ref,
		})
	}
	return withFormat(result)
}
