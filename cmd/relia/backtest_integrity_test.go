package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBacktestFailsClosedForNonRedactedExperienceShard(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `"redaction_status":"applied"`, `"redaction_status":"not_applicable"`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "redaction_safety_failed" || !strings.Contains(result.Errors[0].Message, "redaction_status") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if result.RedactionStatus != "failed_closed" {
		t.Fatalf("redaction_status = %q, want failed_closed", result.RedactionStatus)
	}
}

func TestBacktestFailsClosedForNonPrivateExperienceShard(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `"share_scope":"private"`, `"share_scope":"org"`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "redaction_safety_failed" || !strings.Contains(result.Errors[0].Message, "share_scope") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if result.RedactionStatus != "failed_closed" {
		t.Fatalf("redaction_status = %q, want failed_closed", result.RedactionStatus)
	}
}

func TestBacktestRejectsNonRepoRelativeExperiencePaths(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `"paths":["packages/billing/invoice.py"]`, `"paths":["../secrets.txt"]`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitValidation {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "artifact_contract_validation_failed" || !strings.Contains(result.Errors[0].Message, "repo-relative") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestBacktestRejectsNonCanonicalExperienceProvenanceURLs(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `https://github.com/acme/billing-service/pull/101`, `https://github.com/acme/billing-service/pull/101?from=shard`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "provenance_integrity_failed" || !strings.Contains(result.Errors[0].Message, "canonical") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestBacktestRejectsExperienceProvenanceWithoutMatchingPRURL(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `https://github.com/acme/billing-service/pull/101`, `https://github.com/acme/billing-service/pull/102`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "provenance_integrity_failed" || !strings.Contains(result.Errors[0].Message, "action.pr") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestBacktestAcceptsCanonicalNonPRProvenanceAndDerivesCitation(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","check_run_url":"https://github.com/acme/billing-service/actions/runs/991"}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","check_run_url":"https://github.com/acme/billing-service/actions/runs/992"}`,
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
	if len(report.Citations) != 2 ||
		report.Citations[0].URL != "https://github.com/acme/billing-service/pull/101" ||
		report.Citations[1].URL != "https://github.com/acme/billing-service/pull/102" {
		t.Fatalf("citations = %#v, want derived PR citations for non-PR provenance", report.Citations)
	}
}

func TestBacktestRejectsExperienceProvenanceFromDifferentRepo(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `https://github.com/acme/billing-service/pull/101`, `https://github.com/other-org/other-repo/actions/runs/991`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "provenance_integrity_failed" || !strings.Contains(result.Errors[0].Message, "repo") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestBacktestRejectsPullSubpageProvenanceForDifferentPR(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, `{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	shardPath := filepath.Join(tempDir, ".relia", "experiences", "2026-01.jsonl")
	replaceInFile(t, shardPath, `https://github.com/acme/billing-service/pull/101`, `https://github.com/acme/billing-service/pull/102/files`)

	stdout, stderr, code = runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)

	if code != ExitProvenanceIntegrity {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "provenance_integrity_failed" || !strings.Contains(result.Errors[0].Message, "action.pr") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestBacktestPairsCurrentWithAnyEarlierConfirmedRecurrence(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/tax.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
		`{"experience_id":"exp_0003","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-20T10:00:00Z","pr":103,"commit":"abc003","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/103"]}`,
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
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.Summary.PossibleRecurrenceCount != 1 {
		t.Fatalf("recurrence counts confirmed=%d possible=%d, report=%#v", report.Summary.ConfirmedRecurrenceCount, report.Summary.PossibleRecurrenceCount, report)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.PriorExperienceID != "exp_0001" || confirmed.CurrentExperienceID != "exp_0003" {
		t.Fatalf("confirmed pair = %#v, want current exp_0003 paired with earlier overlapping exp_0001", confirmed)
	}
	possible := report.PossibleRecurrences[0]
	if possible.PriorExperienceID != "exp_0001" || possible.CurrentExperienceID != "exp_0002" {
		t.Fatalf("possible pair = %#v, want disjoint middle occurrence reported separately", possible)
	}
	if report.HeadlineERR != 0.3333 {
		t.Fatalf("headline_err = %.4f, want confirmed-only 1/3", report.HeadlineERR)
	}
}

func TestBacktestIndexesHumanFailuresAsPriorRecurrenceEvidence(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_human_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"human","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_agent_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
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
	if report.Summary.HumanFailureExcludedCount != 1 || report.Summary.AgentFailureDenominator != 1 {
		t.Fatalf("summary = %#v, want one human prior excluded and one agent denominator", report.Summary)
	}
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.HeadlineERR != 1 {
		t.Fatalf("confirmed=%d headline=%.4f, want human-prior recurrence over one agent failure", report.Summary.ConfirmedRecurrenceCount, report.HeadlineERR)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.PriorExperienceID != "exp_human_0001" || confirmed.CurrentExperienceID != "exp_agent_0002" {
		t.Fatalf("confirmed pair = %#v, want agent failure repeating prior human failure", confirmed)
	}
}

func TestBacktestSkipsFlakeDiscountedHumanPriors(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_human_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"human","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","flake_discount":1.0,"provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_agent_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
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
	if report.Summary.HumanFailureExcludedCount != 1 || report.Summary.AgentFailureDenominator != 1 {
		t.Fatalf("summary = %#v, want one excluded human failure and one agent denominator", report.Summary)
	}
	if report.Summary.ConfirmedRecurrenceCount != 0 || report.HeadlineERR != 0 {
		t.Fatalf("confirmed=%d headline=%.4f, want flaky human prior skipped", report.Summary.ConfirmedRecurrenceCount, report.HeadlineERR)
	}
}

func TestBacktestDoesNotPairSamePRAsRecurrence(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze_a","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-02T10:00:00Z","pr":101,"commit":"abc001-rerun","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze_a","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0003","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_time_freeze_a","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
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
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.Summary.PossibleRecurrenceCount != 0 {
		t.Fatalf("recurrence counts confirmed=%d possible=%d, report=%#v", report.Summary.ConfirmedRecurrenceCount, report.Summary.PossibleRecurrenceCount, report)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.CurrentPR == confirmed.PriorPR {
		t.Fatalf("confirmed pair = %#v, same PR should not count as recurrence", confirmed)
	}
	if confirmed.CurrentExperienceID != "exp_0003" || confirmed.PriorPR != 101 {
		t.Fatalf("confirmed pair = %#v, want later PR paired against prior different PR", confirmed)
	}
}

func TestBacktestGroupsEquivalentCanonicalSignatureFields(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_pytest","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_go_test","signature_class":"test_failure","check_name":"go-test-rerun","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
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
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.HeadlineERR != 0.5 {
		t.Fatalf("confirmed=%d headline=%.4f, report=%#v", report.Summary.ConfirmedRecurrenceCount, report.HeadlineERR, report)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.PriorExperienceID != "exp_0001" || confirmed.CurrentExperienceID != "exp_0002" {
		t.Fatalf("confirmed pair = %#v, want canonical class/key grouping despite different generated ids", confirmed)
	}
}

func TestBacktestTopRepeatedMistakesAggregateByMatchedSignature(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_clock","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_clock","signature_class":"test_failure","check_name":"go-test-rerun","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
		`{"experience_id":"exp_0003","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-20T10:00:00Z","pr":103,"commit":"abc003","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_rspec","signature_class":"test_failure","check_name":"rspec-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/103"]}`,
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
	if report.Summary.ConfirmedRecurrenceCount != 2 {
		t.Fatalf("confirmed recurrences = %d, report=%#v", report.Summary.ConfirmedRecurrenceCount, report)
	}
	if len(report.TopRepeatedMistakes) != 1 {
		t.Fatalf("top repeated mistakes = %#v, want one matched-signature aggregate", report.TopRepeatedMistakes)
	}
	mistake := report.TopRepeatedMistakes[0]
	if mistake.SignatureID != "class_key:test_failure:tests/billing/test_invoice.py::test_clock" || mistake.RepeatCount != 2 {
		t.Fatalf("top repeated mistake = %#v, want matched class/key count 2", mistake)
	}
	if len(mistake.PRs) != 3 || mistake.PRs[0] != 101 || mistake.PRs[1] != 102 || mistake.PRs[2] != 103 {
		t.Fatalf("top repeated mistake PRs = %#v, want all matched PRs", mistake.PRs)
	}
	if !stringSlicesEqual(mistake.ExperienceIDs, []string{"exp_0001", "exp_0002", "exp_0003"}) {
		t.Fatalf("top repeated mistake experiences = %#v", mistake.ExperienceIDs)
	}
}

func TestBacktestGroupsClassKeyEvenWithDifferentMessageFingerprints(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","message":"expected 2026-01-01 but got 2025-12-31","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","message":"expected 2026-01-10 but got 2026-01-09","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
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
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.HeadlineERR != 0.5 {
		t.Fatalf("confirmed=%d headline=%.4f, report=%#v", report.Summary.ConfirmedRecurrenceCount, report.HeadlineERR, report)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.PriorExperienceID != "exp_0001" || confirmed.CurrentExperienceID != "exp_0002" {
		t.Fatalf("confirmed pair = %#v, want class/key recurrence despite different message fingerprints", confirmed)
	}
}

func TestBacktestGroupsEquivalentMessageFingerprint(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputPath := filepath.Join(tempDir, "fixtures", "outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0001","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-01T10:00:00Z","pr":101,"commit":"abc001","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_invoice","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","message":"expected 2026-01-01 but got local timezone drift","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0002","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-01-10T10:00:00Z","pr":102,"commit":"abc002","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_invoice_rerun","signature_class":"runtime_failure","check_name":"pytest-billing","signature_key":"runtime-clock-drift","extraction_confidence":"structured","message":"expected 2026-01-01 but got local timezone drift","provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
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
	if report.Summary.ConfirmedRecurrenceCount != 1 || report.HeadlineERR != 0.5 {
		t.Fatalf("confirmed=%d headline=%.4f, report=%#v", report.Summary.ConfirmedRecurrenceCount, report.HeadlineERR, report)
	}
	confirmed := report.ConfirmedRecurrences[0]
	if confirmed.PriorExperienceID != "exp_0001" || confirmed.CurrentExperienceID != "exp_0002" {
		t.Fatalf("confirmed pair = %#v, want message-fingerprint grouping despite different generated ids and classes", confirmed)
	}
}

func TestBacktestExplicitEnabledGateEvaluatesThreshold(t *testing.T) {
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
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), `gate:
  enabled: false`, `gate:
  enabled: true
  max_error_recurrence_rate: 0.0`)
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
	if code != ExitGate {
		t.Fatalf("backtest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || result.Errors[0].Type != "recurrence_gate_failed" {
		t.Fatalf("errors = %#v", result.Errors)
	}
	report := decodeBacktestReportFromResult(t, result)
	if !report.Gate.Enabled || report.Gate.Status != "fail" || report.Gate.Threshold == nil || *report.Gate.Threshold != 0 {
		t.Fatalf("gate = %#v, want enabled failing threshold", report.Gate)
	}
	gateJSON, err := json.Marshal(report.Gate)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(gateJSON, []byte(`"threshold":0`)) {
		t.Fatalf("gate JSON = %s, want zero threshold preserved", gateJSON)
	}
	baselineContent, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(baselineContent) != originalBaseline {
		t.Fatalf("baseline was overwritten by failing gate:\n%s", baselineContent)
	}
}

func TestBacktestRepeatedRunsUseStableReportIDAndArtifacts(t *testing.T) {
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

	firstStdout, firstStderr, firstCode := runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	secondStdout, secondStderr, secondCode := runForTest(t, []string{"--json", "backtest", "--window", "180d"}, false)
	if firstCode != ExitSuccess || secondCode != ExitSuccess {
		t.Fatalf("backtest codes = %d/%d stderr=%q/%q stdout=%q/%q", firstCode, secondCode, firstStderr, secondStderr, firstStdout, secondStdout)
	}
	firstResult := decodeResult(t, firstStdout)
	secondResult := decodeResult(t, secondStdout)
	firstReport := decodeBacktestReportFromResult(t, firstResult)
	secondReport := decodeBacktestReportFromResult(t, secondResult)
	if firstReport.ReportID != secondReport.ReportID {
		t.Fatalf("report_id changed across repeated runs: %q then %q", firstReport.ReportID, secondReport.ReportID)
	}
	firstPath, _ := firstResult.Data["json_report_path"].(string)
	secondPath, _ := secondResult.Data["json_report_path"].(string)
	if firstPath != secondPath {
		t.Fatalf("json report path changed: %q then %q", firstPath, secondPath)
	}
	firstContent, err := os.ReadFile(filepath.Join(tempDir, filepath.FromSlash(firstPath)))
	if err != nil {
		t.Fatal(err)
	}
	secondContent, err := os.ReadFile(filepath.Join(tempDir, filepath.FromSlash(secondPath)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstContent, secondContent) {
		t.Fatalf("json report content changed across repeated runs:\nfirst=%s\nsecond=%s", firstContent, secondContent)
	}
}
