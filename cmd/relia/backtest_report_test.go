package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	backtestdoc "github.com/Clyra-AI/relia/internal/backtest"
)

func TestBacktestComputesConservativeERRWithFlakesPossibleAndStaleBaseline(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py","tests/billing/test_invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py","tests/billing/test_invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
		`{"experience_id":"exp_0003","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-20T10:00:00Z","pr":103,"commit":"abc003","paths":["packages/billing/invoice.py","tests/billing/test_invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"log_parsed_low","provenance_urls":["https://github.com/acme/billing-service/pull/103"]}`,
		`{"experience_id":"exp_0101","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-02-01T10:00:00Z","pr":201,"commit":"abc101","paths":["packages/notifications/logging.py","tests/notifications/test_retry.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_external_retry","signature_class":"test_failure","check_name":"pytest-notifications","signature_key":"tests/notifications/test_retry.py::test_retry","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/201"]}`,
		`{"experience_id":"exp_0102","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-02-02T10:00:00Z","pr":202,"commit":"abc102","paths":["packages/worker/retry_queue.py","tests/notifications/test_retry.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_external_retry","signature_class":"test_failure","check_name":"pytest-notifications","signature_key":"tests/notifications/test_retry.py::test_retry","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/202"]}`,
		`{"experience_id":"exp_0103","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-02-03T10:00:00Z","pr":203,"commit":"abc103","paths":["packages/notifications/client.py","tests/notifications/test_retry.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_external_retry","signature_class":"test_failure","check_name":"pytest-notifications","signature_key":"tests/notifications/test_retry.py::test_retry","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/203"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json"), `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.1,
  "metadata": {
    "source_artifact_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
  }
}
`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	report := decodeBacktestReportFromResult(t, result)
	if result.RedactionStatus != "applied" {
		t.Fatalf("redaction_status = %q, want applied", result.RedactionStatus)
	}
	if report.ObjectType != "relia.recurrence_report" || report.SchemaVersion != commandSchemaVersion {
		t.Fatalf("report contract = %#v", report)
	}
	if report.Metrics.PRsAnalyzed != 6 || report.Metrics.AgentAttributedPRs != 6 {
		t.Fatalf("metrics = %#v, want six analyzed and agent-attributed PRs", report.Metrics)
	}
	if report.Metrics.AgentFailuresByOutcomeKind["ci_failure"] != 6 {
		t.Fatalf("agent failure breakdown = %#v", report.Metrics.AgentFailuresByOutcomeKind)
	}
	if report.Summary.AgentFailureDenominator != 6 {
		t.Fatalf("denominator = %d, want 6 total agent-attributed failures including flake-discounted rows", report.Summary.AgentFailureDenominator)
	}
	if report.Summary.ConfirmedRecurrenceCount != 1 || len(report.ConfirmedRecurrences) != 1 {
		t.Fatalf("confirmed recurrences = %#v", report.ConfirmedRecurrences)
	}
	if report.Summary.PossibleRecurrenceCount != 1 || len(report.PossibleRecurrences) != 1 {
		t.Fatalf("possible recurrences = %#v", report.PossibleRecurrences)
	}
	if report.HeadlineERR != 0.1667 {
		t.Fatalf("headline_err = %.4f, want confirmed-only numerator over 6 agent-attributed failures", report.HeadlineERR)
	}
	if report.ConfirmedRecurrences[0].PriorExperienceID != "exp_0001" || report.ConfirmedRecurrences[0].CurrentExperienceID != "exp_0002" {
		t.Fatalf("confirmed pair = %#v", report.ConfirmedRecurrences[0])
	}
	if report.ConfirmedRecurrences[0].PriorURL == "" || report.ConfirmedRecurrences[0].CurrentURL == "" || len(report.ConfirmedRecurrences[0].Refs) != 2 {
		t.Fatalf("confirmed pair missing resolvable links and refs: %#v", report.ConfirmedRecurrences[0])
	}
	if report.PossibleRecurrences[0].Confidence != "possible" || !strings.Contains(report.PossibleRecurrences[0].Reason, "excluded") {
		t.Fatalf("possible pair = %#v", report.PossibleRecurrences[0])
	}
	if report.Summary.FlakeDiscountedCount != 3 || len(report.FlakeDiscounts) != 3 {
		t.Fatalf("flake discounts = %#v", report.FlakeDiscounts)
	}
	if report.Baseline.Status != "stale" || !report.Baseline.Stale {
		t.Fatalf("baseline = %#v, want stale", report.Baseline)
	}
	if report.Gate.Enabled || report.Gate.Status != "off" {
		t.Fatalf("gate = %#v, want off by default", report.Gate)
	}
	if len(report.TopRepeatedMistakes) != 1 ||
		report.TopRepeatedMistakes[0].SignatureID != "class_key:test_failure:tests/billing/test_invoice.py::test_clock" ||
		report.TopRepeatedMistakes[0].RepeatCount != 1 ||
		!stringSlicesEqual(report.TopRepeatedMistakes[0].ExperienceIDs, []string{"exp_0001", "exp_0002"}) {
		t.Fatalf("top repeated mistakes = %#v", report.TopRepeatedMistakes)
	}
	if report.OperatorFeedback.Summary == "" ||
		!strings.Contains(report.OperatorFeedback.ConservativeMatchingNote, "confirmed") ||
		report.OperatorFeedback.NextCommand != "relia distill --format json" {
		t.Fatalf("operator feedback = %#v", report.OperatorFeedback)
	}
	if report.Badge.Label != "Relia" ||
		report.Badge.Message != "ERR 16.7%" ||
		report.Badge.Status != "current" ||
		report.Badge.Stale ||
		report.Badge.Color != "yellow" ||
		!strings.Contains(report.Badge.Reason, "ingest metadata") {
		t.Fatalf("badge = %#v", report.Badge)
	}
	if report.Metadata["last_ingest_at"] == "" || report.Metadata["merged_prs_since_last_ingest"] != float64(0) {
		t.Fatalf("badge freshness metadata = %#v", report.Metadata)
	}
	assertReportDiagnosticTypes(t, report.Diagnostics, []string{
		"memory_source_verified",
		"possible_recurrences_excluded",
		"flake_discounts_visible",
		"stale_baseline",
	})
	jsonPath, _ := result.Data["json_report_path"].(string)
	htmlPath, _ := result.Data["html_report_path"].(string)
	if jsonPath == "" || htmlPath == "" {
		t.Fatalf("report artifact paths missing from result data: %#v", result.Data)
	}
	if result.Data["report_path"] != htmlPath ||
		result.Data["error_recurrence_rate"] != report.HeadlineERR ||
		result.Data["baseline_ref"] != ".relia/baselines/error-recurrence-baseline.json" {
		t.Fatalf("result data did not expose report metrics and refs: %#v", result.Data)
	}
	jsonContent, err := os.ReadFile(filepath.Join(tempDir, filepath.FromSlash(jsonPath)))
	if err != nil {
		t.Fatalf("read json report: %v", err)
	}
	if !bytes.Contains(jsonContent, []byte(`"object_type": "relia.recurrence_report"`)) {
		t.Fatalf("json report missing recurrence object:\n%s", jsonContent)
	}
	htmlContent, err := os.ReadFile(filepath.Join(tempDir, filepath.FromSlash(htmlPath)))
	if err != nil {
		t.Fatalf("read html report: %v", err)
	}
	if !bytes.Contains(htmlContent, []byte("Possible Recurrences")) {
		t.Fatalf("html report missing possible recurrence section:\n%s", htmlContent)
	}
	if !bytes.Contains(htmlContent, []byte("Top Repeated Mistakes")) ||
		!bytes.Contains(htmlContent, []byte("Badge: Relia ERR 16.7%")) {
		t.Fatalf("html report missing operator summary and badge:\n%s", htmlContent)
	}
}

func TestBuildReportBadgeComputesFreshness(t *testing.T) {
	report := recurrenceReport{
		ReportID:    "backtest_fresh",
		Window:      recurrenceWindow{End: "2026-01-20T00:00:00Z"},
		Summary:     recurrenceSummary{HeadlineERRPercent: "4.1%"},
		HeadlineERR: 0.041,
		Metadata: map[string]any{
			"last_ingest_at":               "2026-06-20T00:00:00Z",
			"merged_prs_since_last_ingest": 0,
		},
	}
	now := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	badge := backtestdoc.BuildReportBadgeAt(report, now)
	if badge.Status != "current" || badge.Stale || badge.Message != "ERR 4.1%" || badge.Color != "brightgreen" {
		t.Fatalf("fresh badge = %#v, want current", badge)
	}
	if !strings.Contains(badge.Reason, "ingest metadata") {
		t.Fatalf("fresh badge reason = %q", badge.Reason)
	}

	report.Metadata["last_ingest_at"] = "2026-05-29T00:00:00Z"
	badge = backtestdoc.BuildReportBadgeAt(report, now)
	if badge.Status != "stale" || !badge.Stale || badge.Message != "ERR 4.1% stale" || badge.Color != "lightgrey" {
		t.Fatalf("old badge = %#v, want stale", badge)
	}
	if !strings.Contains(badge.Reason, "Last ingest exceeds") {
		t.Fatalf("old badge reason = %q", badge.Reason)
	}

	report.Metadata = map[string]any{
		"merged_prs_since_last_ingest": 0,
	}
	badge = backtestdoc.BuildReportBadgeAt(report, now)
	if badge.Status != "stale" || !badge.Stale {
		t.Fatalf("missing ingest badge = %#v, want stale", badge)
	}
	if !strings.Contains(badge.Reason, "Ingest freshness is unavailable") {
		t.Fatalf("missing ingest badge reason = %q", badge.Reason)
	}
}

func TestBuildReportBadgeComputesActivityStaleness(t *testing.T) {
	report := recurrenceReport{
		ReportID: "backtest_activity",
		Window:   recurrenceWindow{End: "2026-06-20T00:00:00Z"},
		Summary:  recurrenceSummary{HeadlineERRPercent: "4.1%"},
		Metadata: map[string]any{
			"last_ingest_at":               "2026-06-20T00:00:00Z",
			"merged_prs_since_last_ingest": float64(21),
		},
	}
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)

	badge := backtestdoc.BuildReportBadgeAt(report, now)
	if badge.Status != "stale" || !badge.Stale || badge.Message != "ERR 4.1% stale" {
		t.Fatalf("activity badge = %#v, want stale", badge)
	}
	if !strings.Contains(badge.Reason, "20 PRs") {
		t.Fatalf("activity badge reason = %q", badge.Reason)
	}

	report.Metadata = map[string]any{
		"last_ingest_at": "2026-06-20T00:00:00Z",
	}
	badge = backtestdoc.BuildReportBadgeAt(report, now)
	if badge.Status != "stale" || !badge.Stale {
		t.Fatalf("missing activity badge = %#v, want stale", badge)
	}
	if !strings.Contains(badge.Reason, "activity freshness is unavailable") {
		t.Fatalf("missing activity badge reason = %q", badge.Reason)
	}

	report.Metadata = map[string]any{
		"last_ingest_at":               "2026-06-20T00:00:00Z",
		"merged_prs_since_last_ingest": json.Number("-1"),
	}
	badge = backtestdoc.BuildReportBadgeAt(report, now)
	if badge.Status != "stale" || !badge.Stale {
		t.Fatalf("negative activity badge = %#v, want stale", badge)
	}
	if !strings.Contains(badge.Reason, "activity freshness is unavailable") {
		t.Fatalf("negative activity badge reason = %q", badge.Reason)
	}
}

func TestBacktestCommandResultCountsAgentAttributedExperiencesSeparatelyFromPRs(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_invoice_tax","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_tax","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-02T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/payment.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"review_correction","terminal_state":"corrected","signature_id":"sig_payment_rounding","signature_class":"review_correction","check_name":"review","signature_key":"packages/billing/payment.py::rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	report := decodeBacktestReportFromResult(t, result)
	if got := int(result.Data["experiences_agent_attributed"].(float64)); got != 2 {
		t.Fatalf("experiences_agent_attributed = %d, want two agent-attributed records", got)
	}
	if got := int(result.Data["agent_attributed_prs"].(float64)); got != 1 {
		t.Fatalf("agent_attributed_prs = %d, want one unique agent-attributed PR", got)
	}
	if report.Metrics.AgentAttributedExperiences != 2 || report.Metrics.AgentAttributedPRs != 1 {
		t.Fatalf("metrics = %#v, want separate experience and PR counts", report.Metrics)
	}
}

func TestBacktestInteractiveOutputShowsOperatorSummaryAndBadge(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"backtest", "--window", "180d"}, true)

	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	if json.Valid([]byte(stdout)) {
		t.Fatalf("interactive backtest output should be human-readable, got JSON: %s", stdout)
	}
	for _, want := range []string{
		"PRs analyzed: 2",
		"Confirmed recurrences: 1",
		"Top repeated mistakes:",
		"Error recurrence rate: 50.0%",
		"Badge: Relia ERR 50.0%",
		"Report: .relia/reports/",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("interactive backtest output missing %q:\n%s", want, stdout)
		}
	}
}

func TestBacktestAutoFlakeUsesCanonicalSignatureKeys(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_pytest","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/worker/clock.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_go_test","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
		`{"experience_id":"exp_0003","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-20T10:00:00Z","pr":103,"commit":"abc003","paths":["packages/notifications/clock.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_actions","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/103"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.Summary.AgentFailureDenominator != 3 || report.Summary.FlakeDiscountedCount != 3 || len(report.FlakeDiscounts) != 3 {
		t.Fatalf("summary = %#v flakes = %#v, want three canonical-key flakes retained in denominator", report.Summary, report.FlakeDiscounts)
	}
	for _, flake := range report.FlakeDiscounts {
		if len(flake.SupportingPRs) != 2 || len(flake.SupportingRefs) != 2 {
			t.Fatalf("flake = %#v, want canonical-key supporting PRs and refs despite different generated ids", flake)
		}
	}
	if report.Summary.ConfirmedRecurrenceCount != 0 || report.Summary.PossibleRecurrenceCount != 0 || report.HeadlineERR != 0 {
		t.Fatalf("recurrences confirmed=%d possible=%d headline=%.4f, want flakes excluded from recurrence scoring", report.Summary.ConfirmedRecurrenceCount, report.Summary.PossibleRecurrenceCount, report.HeadlineERR)
	}
}

func TestBacktestBaselineAcceptsSummaryHeadlineERR(t *testing.T) {
	tempDir := t.TempDir()
	baselinePath := filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json")
	currentWindow := recurrenceWindow{Start: "2026-01-01T00:00:00Z", End: "2026-01-31T00:00:00Z"}
	writeFileForTest(t, baselinePath, `{
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
`)

	baseline, commandErr := compareBacktestBaseline(tempDir, ".relia/baselines/error-recurrence-baseline.json", 0.5, "sha256:baseline", currentWindow)

	if commandErr != nil {
		t.Fatalf("compare baseline returned error: %#v", commandErr)
	}
	if baseline.Status != "current" || baseline.Stale {
		t.Fatalf("baseline status = %#v, want current", baseline)
	}
	if baseline.HeadlineERR != 0.25 || baseline.Delta != 0.25 {
		t.Fatalf("baseline values = %#v, want headline 0.25 and delta 0.25", baseline)
	}
}

func TestBacktestBaselineMarksWindowMismatchStale(t *testing.T) {
	tempDir := t.TempDir()
	baselinePath := filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json")
	writeFileForTest(t, baselinePath, `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.25,
  "window": {
    "start": "2026-01-01T00:00:00Z",
    "end": "2026-06-29T00:00:00Z"
  },
  "metadata": {
    "source_artifact_digest": "sha256:baseline"
  }
}
`)

	currentWindow := recurrenceWindow{Start: "2026-06-01T00:00:00Z", End: "2026-06-29T00:00:00Z"}
	baseline, commandErr := compareBacktestBaseline(tempDir, ".relia/baselines/error-recurrence-baseline.json", 0.5, "sha256:baseline", currentWindow)

	if commandErr != nil {
		t.Fatalf("compare baseline returned error: %#v", commandErr)
	}
	if baseline.Status != "stale" || !baseline.Stale || !strings.Contains(baseline.Reason, "window") {
		t.Fatalf("baseline status = %#v, want stale window mismatch", baseline)
	}
	if baseline.HeadlineERR != 0.25 || baseline.Delta != 0.25 {
		t.Fatalf("baseline values = %#v, want headline 0.25 and delta 0.25", baseline)
	}
}

func TestBacktestBaselineJSONPreservesZeroMetrics(t *testing.T) {
	current := baselineComparison{
		Status:      "current",
		Path:        ".relia/baselines/error-recurrence-baseline.json",
		HeadlineERR: 0,
		Delta:       0,
		Stale:       false,
		Reason:      "Saved baseline was computed from the same source artifact digest.",
	}
	currentJSON, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(currentJSON, []byte(`"headline_err":0`)) || !bytes.Contains(currentJSON, []byte(`"delta":0`)) {
		t.Fatalf("current baseline JSON = %s, want explicit zero metrics", currentJSON)
	}

	missing := baselineComparison{
		Status: "missing",
		Path:   ".relia/baselines/error-recurrence-baseline.json",
		Stale:  false,
		Reason: "No saved ERR baseline exists yet; use --save-baseline after reviewing the report to create one.",
	}
	missingJSON, err := json.Marshal(missing)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(missingJSON, []byte("headline_err")) || bytes.Contains(missingJSON, []byte("delta")) {
		t.Fatalf("missing baseline JSON = %s, want omitted comparison metrics", missingJSON)
	}
}

func TestBacktestReportSerializesEmptyCollectionsAsArrays(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	reportData, ok := result.Data["report"].(map[string]any)
	if !ok {
		t.Fatalf("report data = %#v, want object", result.Data["report"])
	}
	for _, field := range []string{"confirmed_recurrences", "possible_recurrences", "flake_discounts", "attribution_uncertain"} {
		values, ok := reportData[field].([]any)
		if !ok || len(values) != 0 {
			t.Fatalf("report[%s] = %#v, want empty JSON array", field, reportData[field])
		}
	}
	report := decodeBacktestReportFromResult(t, result)
	if report.Summary.AgentFailureDenominator != 1 || report.HeadlineERR != 0 {
		t.Fatalf("report summary = %#v headline=%.4f, want one denominator and zero ERR", report.Summary, report.HeadlineERR)
	}
}

func TestBacktestSaveBaselineReportsFreshCurrentValues(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json"), `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.1,
  "window": {
    "start": "2025-01-01T00:00:00Z",
    "end": "2025-01-31T00:00:00Z"
  },
  "metadata": {
    "source_artifact_digest": "sha256:stale"
  }
}
`)
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d", "--save-baseline"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if report.HeadlineERR != 0.5 {
		t.Fatalf("headline_err = %.4f, want 0.5", report.HeadlineERR)
	}
	if report.Baseline.Status != "saved" || report.Baseline.Stale || report.Baseline.HeadlineERR != report.HeadlineERR || report.Baseline.Delta != 0 {
		t.Fatalf("baseline = %#v, want freshly saved current values", report.Baseline)
	}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Type == "stale_baseline" {
			t.Fatalf("diagnostics retained stale baseline after save: %#v", report.Diagnostics)
		}
	}
	baselineContent, err := os.ReadFile(filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(baselineContent, []byte(`"headline_err": 0.5`)) {
		t.Fatalf("saved baseline missing current headline ERR:\n%s", baselineContent)
	}
}

func TestBacktestBaselineDigestIgnoresIngestFreshnessMetadata(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d", "--save-baseline"}, false)
	if code != ExitSuccess {
		t.Fatalf("save baseline exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	savedReport := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	savedDigest := stringFromAny(savedReport.Metadata["source_artifact_digest"])
	if savedDigest == "" {
		t.Fatalf("saved report missing source digest: %#v", savedReport.Metadata)
	}

	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	shardContent, err := os.ReadFile(shardPath)
	if err != nil {
		t.Fatal(err)
	}
	records := decodeJSONLines(t, string(shardContent))
	lines := make([]string, 0, len(records))
	for _, record := range records {
		metadata := record["metadata"].(map[string]any)
		metadata["last_ingest_at"] = "2026-06-30T14:15:00Z"
		metadata["merged_prs_since_last_ingest"] = float64(7)
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(encoded))
	}
	writeFileForTest(t, shardPath, strings.Join(lines, "\n")+"\n")

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if code != ExitSuccess {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	report := decodeBacktestReportFromResult(t, decodeResult(t, stdout))
	if got := stringFromAny(report.Metadata["source_artifact_digest"]); got != savedDigest {
		t.Fatalf("source digest = %q, want unchanged %q", got, savedDigest)
	}
	if report.Baseline.Status != "current" || report.Baseline.Stale {
		t.Fatalf("baseline = %#v, want current after freshness-only metadata change", report.Baseline)
	}
}

func TestBacktestRollsBackBaselineWhenReportWriteFails(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	baselinePath := filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json")
	originalBaseline := `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.01,
  "metadata": {
    "source_artifact_digest": "sha256:accepted"
  }
}
`
	writeFileForTest(t, baselinePath, originalBaseline)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "reports"), "not a directory\n")

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d", "--save-baseline"}, false)

	if code == ExitSuccess {
		t.Fatalf("backtest unexpectedly succeeded, stderr = %q, stdout = %q", stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "backtest report directory") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	baselineContent, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(baselineContent) != originalBaseline {
		t.Fatalf("baseline was not rolled back after report write failure:\n%s", baselineContent)
	}
}

func TestBacktestRemovesJSONReportWhenHTMLWriteFails(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	probeStdout, probeStderr, probeCode := runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if probeCode != ExitSuccess {
		t.Fatalf("probe backtest exit code = %d, stderr = %q, stdout = %q", probeCode, probeStderr, probeStdout)
	}
	probeResult := decodeResult(t, probeStdout)
	jsonRel, _ := probeResult.Data["json_report_path"].(string)
	htmlRel, _ := probeResult.Data["html_report_path"].(string)
	jsonPath := filepath.Join(tempDir, filepath.FromSlash(jsonRel))
	htmlPath := filepath.Join(tempDir, filepath.FromSlash(htmlRel))
	if err := os.Remove(jsonPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(htmlPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(htmlPath, 0o755); err != nil {
		t.Fatal(err)
	}
	baselinePath := filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json")
	originalBaseline := `{
  "object_type": "relia.err_baseline",
  "schema_version": "1.0",
  "headline_err": 0.01,
  "metadata": {
    "source_artifact_digest": "sha256:accepted"
  }
}
`
	writeFileForTest(t, baselinePath, originalBaseline)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d", "--save-baseline"}, false)

	if code == ExitSuccess {
		t.Fatalf("backtest unexpectedly succeeded, stderr = %q, stdout = %q", stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "HTML recurrence report") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Fatalf("partial JSON report still exists after HTML failure: %v", err)
	}
	baselineContent, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(baselineContent) != originalBaseline {
		t.Fatalf("baseline was not rolled back after HTML report failure:\n%s", baselineContent)
	}
}

func TestBacktestPreservesExistingJSONReportWhenHTMLWriteFails(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	probeStdout, probeStderr, probeCode := runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if probeCode != ExitSuccess {
		t.Fatalf("probe backtest exit code = %d, stderr = %q, stdout = %q", probeCode, probeStderr, probeStdout)
	}
	probeResult := decodeResult(t, probeStdout)
	jsonRel, _ := probeResult.Data["json_report_path"].(string)
	htmlRel, _ := probeResult.Data["html_report_path"].(string)
	jsonPath := filepath.Join(tempDir, filepath.FromSlash(jsonRel))
	htmlPath := filepath.Join(tempDir, filepath.FromSlash(htmlRel))
	originalJSON := []byte("{\"object_type\":\"prior.recurrence_report\"}\n")
	if err := os.WriteFile(jsonPath, originalJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(htmlPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(htmlPath, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code == ExitSuccess {
		t.Fatalf("backtest unexpectedly succeeded, stderr = %q, stdout = %q", stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "HTML recurrence report") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	content, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, originalJSON) {
		t.Fatalf("existing JSON report changed after HTML failure:\n%s", content)
	}
}

func TestBacktestFailsClosedWhenBaselinePathIsDirectory(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	baselinePath := filepath.Join(tempDir, ".relia", "baselines", "error-recurrence-baseline.json")
	if err := os.MkdirAll(baselinePath, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d", "--save-baseline"}, false)

	if code == ExitSuccess {
		t.Fatalf("backtest unexpectedly succeeded, stderr = %q, stdout = %q", stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "ERR baseline") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	info, err := os.Stat(baselinePath)
	if err != nil || !info.IsDir() {
		t.Fatalf("baseline path = info:%#v err:%v, want directory left intact", info, err)
	}
}
