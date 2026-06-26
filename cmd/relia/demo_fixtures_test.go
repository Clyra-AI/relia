package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

type demoRepoRef struct {
	Provider string `json:"provider"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
}

type demoPRIndex struct {
	ObjectType    string                 `json:"object_type"`
	SchemaVersion string                 `json:"schema_version"`
	Repo          demoRepoRef            `json:"repo"`
	PRs           []demoPR               `json:"prs"`
	Metadata      map[string]interface{} `json:"metadata"`
}

type demoPR struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}

type demoOutcomeFixture struct {
	ExperienceID          string   `json:"experience_id"`
	RecordedAt            string   `json:"recorded_at"`
	PR                    int      `json:"pr"`
	ActorKind             string   `json:"actor_kind"`
	AttributionMethod     string   `json:"attribution_method"`
	AttributionConfidence float64  `json:"attribution_confidence"`
	OutcomeKind           string   `json:"outcome_kind"`
	SignatureID           string   `json:"signature_id"`
	SignatureKey          string   `json:"signature_key"`
	ExtractionConfidence  string   `json:"extraction_confidence"`
	FlakeDiscount         float64  `json:"flake_discount"`
	ProvenanceURLs        []string `json:"provenance_urls"`
}

type demoBacktestReport struct {
	ObjectType    string `json:"object_type"`
	SchemaVersion string `json:"schema_version"`
	ReportID      string `json:"report_id"`
	SourceRepo    struct {
		Provider string `json:"provider"`
		Owner    string `json:"owner"`
		Name     string `json:"name"`
	} `json:"source_repo"`
	SourceArtifacts []string `json:"source_artifacts"`
	Summary         struct {
		SeededPRs                 int     `json:"seeded_prs"`
		AgentFailureDenominator   int     `json:"agent_failure_denominator"`
		ConfirmedRecurrenceCount  int     `json:"confirmed_recurrence_count"`
		PossibleRecurrenceCount   int     `json:"possible_recurrence_count"`
		HeadlineERR               float64 `json:"headline_err"`
		HeadlineERRPercent        string  `json:"headline_err_percent"`
		FlakeDiscountedCount      int     `json:"flake_discounted_count"`
		AttributionUncertainCount int     `json:"attribution_uncertain_count"`
	} `json:"summary"`
	ConfirmedRecurrences []demoRecurrencePair `json:"confirmed_recurrences"`
	FlakeDiscounts       []demoFlakeDiscount  `json:"flake_discounts"`
	RuleCandidates       []demoRuleCandidate  `json:"rule_candidates"`
	Citations            []demoCitation       `json:"citations"`
	RedactionStatus      string               `json:"redaction_status"`
}

type demoRecurrencePair struct {
	CurrentExperienceID string `json:"current_experience_id"`
	PriorExperienceID   string `json:"prior_experience_id"`
	CurrentPR           int    `json:"current_pr"`
	PriorPR             int    `json:"prior_pr"`
	Confidence          string `json:"confidence"`
}

type demoFlakeDiscount struct {
	PR            int     `json:"pr"`
	SignatureID   string  `json:"signature_id"`
	FlakeDiscount float64 `json:"flake_discount"`
	DraftsRule    bool    `json:"drafts_rule"`
}

type demoRuleCandidate struct {
	RuleID      string `json:"rule_id"`
	Kind        string `json:"kind"`
	SignatureID string `json:"signature_id"`
	EvidencePRs []int  `json:"evidence_prs"`
	HeldFixPR   int    `json:"held_fix_pr,omitempty"`
}

type demoCitation struct {
	PR  int    `json:"pr"`
	URL string `json:"url"`
}

type demoAttributionSample struct {
	ObjectType    string `json:"object_type"`
	SchemaVersion string `json:"schema_version"`
	Metrics       struct {
		LabeledCases       int     `json:"labeled_cases"`
		EvaluatedCases     int     `json:"evaluated_cases"`
		CorrectPredictions int     `json:"correct_predictions"`
		ExcludedUncertain  int     `json:"excluded_uncertain"`
		Precision          float64 `json:"precision"`
		PrecisionThreshold float64 `json:"precision_threshold"`
	} `json:"metrics"`
	Cases []demoAttributionCase `json:"cases"`
}

type demoAttributionCase struct {
	CaseID              string  `json:"case_id"`
	PR                  int     `json:"pr"`
	LabeledActorKind    string  `json:"labeled_actor_kind"`
	PredictedActorKind  string  `json:"predicted_actor_kind"`
	PredictedMethod     string  `json:"predicted_method"`
	Confidence          float64 `json:"confidence"`
	IncludedInPrecision bool    `json:"included_in_precision"`
}

type demoRedactionFixtures struct {
	ObjectType    string `json:"object_type"`
	SchemaVersion string `json:"schema_version"`
	Cases         []struct {
		CaseID                string `json:"case_id"`
		StoresRawSecret       bool   `json:"stores_raw_secret"`
		ExpectedArtifactValue string `json:"expected_artifact_value"`
	} `json:"cases"`
}

func TestDemoFixtureCorpusReproducesBacktestReport(t *testing.T) {
	root := findRepoRootForTest(t)
	prIndex := readDemoPRIndex(t, root)
	outcomes := readDemoOutcomes(t, root)
	report := readDemoBacktestReport(t, root)
	prURLs := demoPRURLs(t, prIndex)

	stats, pairs := computeDemoStats(t, outcomes)
	if report.Summary.SeededPRs != len(prIndex.PRs) {
		t.Fatalf("seeded_prs = %d, want %d", report.Summary.SeededPRs, len(prIndex.PRs))
	}
	if report.Summary.AgentFailureDenominator != stats.agentFailureDenominator {
		t.Fatalf("agent_failure_denominator = %d, want %d", report.Summary.AgentFailureDenominator, stats.agentFailureDenominator)
	}
	if report.Summary.ConfirmedRecurrenceCount != stats.confirmedRecurrenceCount {
		t.Fatalf("confirmed_recurrence_count = %d, want %d", report.Summary.ConfirmedRecurrenceCount, stats.confirmedRecurrenceCount)
	}
	if report.Summary.FlakeDiscountedCount != stats.flakeDiscountedCount {
		t.Fatalf("flake_discounted_count = %d, want %d", report.Summary.FlakeDiscountedCount, stats.flakeDiscountedCount)
	}
	if report.Summary.AttributionUncertainCount != stats.attributionUncertainCount {
		t.Fatalf("attribution_uncertain_count = %d, want %d", report.Summary.AttributionUncertainCount, stats.attributionUncertainCount)
	}
	wantERR := float64(stats.confirmedRecurrenceCount) / float64(stats.agentFailureDenominator)
	if math.Abs(report.Summary.HeadlineERR-wantERR) > 0.0001 {
		t.Fatalf("headline_err = %.6f, want %.6f", report.Summary.HeadlineERR, wantERR)
	}
	wantPercent := fmt.Sprintf("%.1f%%", wantERR*100)
	if report.Summary.HeadlineERRPercent != wantPercent {
		t.Fatalf("headline_err_percent = %q, want %q", report.Summary.HeadlineERRPercent, wantPercent)
	}
	assertDemoPairsMatch(t, report.ConfirmedRecurrences, pairs)
	assertDemoReportCitationsResolve(t, report, prURLs)
	assertFlakeDiscountDraftsNoRule(t, report)
	assertDemoStaticReportTextMatchesJSON(t, root, report, prURLs)
}

func TestDemoAttributionPrecisionSampleExcludesUncertain(t *testing.T) {
	root := findRepoRootForTest(t)
	prURLs := demoPRURLs(t, readDemoPRIndex(t, root))
	sample := readDemoAttributionSample(t, root)

	var evaluated, correct, excludedUncertain int
	for _, testCase := range sample.Cases {
		if _, ok := prURLs[testCase.PR]; !ok {
			t.Fatalf("attribution case %s references unknown seeded PR #%d", testCase.CaseID, testCase.PR)
		}
		if testCase.LabeledActorKind == "uncertain" {
			if testCase.PredictedActorKind != "uncertain" || testCase.IncludedInPrecision {
				t.Fatalf("uncertain attribution case was guessed instead of excluded: %#v", testCase)
			}
			excludedUncertain++
			continue
		}
		if !testCase.IncludedInPrecision {
			continue
		}
		if testCase.PredictedActorKind == "uncertain" {
			t.Fatalf("uncertain prediction included in precision: %#v", testCase)
		}
		evaluated++
		if testCase.PredictedActorKind == testCase.LabeledActorKind {
			correct++
		}
	}
	precision := float64(correct) / float64(evaluated)
	if sample.Metrics.LabeledCases != len(sample.Cases) {
		t.Fatalf("labeled_cases = %d, want %d", sample.Metrics.LabeledCases, len(sample.Cases))
	}
	if sample.Metrics.EvaluatedCases != evaluated {
		t.Fatalf("evaluated_cases = %d, want %d", sample.Metrics.EvaluatedCases, evaluated)
	}
	if sample.Metrics.CorrectPredictions != correct {
		t.Fatalf("correct_predictions = %d, want %d", sample.Metrics.CorrectPredictions, correct)
	}
	if sample.Metrics.ExcludedUncertain != excludedUncertain {
		t.Fatalf("excluded_uncertain = %d, want %d", sample.Metrics.ExcludedUncertain, excludedUncertain)
	}
	if math.Abs(sample.Metrics.Precision-precision) > 0.0001 {
		t.Fatalf("precision = %.6f, want %.6f", sample.Metrics.Precision, precision)
	}
	if sample.Metrics.Precision < sample.Metrics.PrecisionThreshold || sample.Metrics.PrecisionThreshold < 0.95 {
		t.Fatalf("precision %.3f below required threshold %.3f", sample.Metrics.Precision, sample.Metrics.PrecisionThreshold)
	}
}

func TestDemoArtifactsAreCustomerSafeAndDoNotContainSeededSecrets(t *testing.T) {
	root := findRepoRootForTest(t)
	fixtures := readDemoRedactionFixtures(t, root)
	for _, testCase := range fixtures.Cases {
		if testCase.StoresRawSecret {
			t.Fatalf("redaction fixture %s stores a raw seeded secret", testCase.CaseID)
		}
		if !strings.HasPrefix(testCase.ExpectedArtifactValue, "[REDACTED:") {
			t.Fatalf("redaction fixture %s expected artifact value is not redacted: %q", testCase.CaseID, testCase.ExpectedArtifactValue)
		}
	}
	scanRoots := []string{
		filepath.Join(root, "examples", "demo"),
		filepath.Join(root, "examples", "reports"),
	}
	for _, scanRoot := range scanRoots {
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			assertCustomerSafeDemoContent(t, filepath.ToSlash(path), string(content))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

type demoComputedStats struct {
	agentFailureDenominator   int
	confirmedRecurrenceCount  int
	flakeDiscountedCount      int
	attributionUncertainCount int
}

func readDemoPRIndex(t *testing.T, root string) demoPRIndex {
	t.Helper()
	var index demoPRIndex
	readJSONFileForTest(t, filepath.Join(root, "examples", "demo", "seeded-repo", "prs.json"), &index)
	if index.ObjectType != "relia.demo_seeded_pr_index" {
		t.Fatalf("unexpected PR index object_type %q", index.ObjectType)
	}
	return index
}

func readDemoOutcomes(t *testing.T, root string) []demoOutcomeFixture {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "examples", "demo", "seeded-repo", "outcomes.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var outcomes []demoOutcomeFixture
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var outcome demoOutcomeFixture
		if err := json.Unmarshal([]byte(line), &outcome); err != nil {
			t.Fatalf("decode demo outcome line %d: %v", lineNumber+1, err)
		}
		outcomes = append(outcomes, outcome)
	}
	if len(outcomes) == 0 {
		t.Fatal("demo outcome corpus is empty")
	}
	return outcomes
}

func readDemoBacktestReport(t *testing.T, root string) demoBacktestReport {
	t.Helper()
	var report demoBacktestReport
	readJSONFileForTest(t, filepath.Join(root, "examples", "reports", "backtest-demo.json"), &report)
	if report.ObjectType != "relia.demo_backtest_report" {
		t.Fatalf("unexpected report object_type %q", report.ObjectType)
	}
	if report.RedactionStatus != "customer_safe" {
		t.Fatalf("redaction_status = %q", report.RedactionStatus)
	}
	return report
}

func readDemoAttributionSample(t *testing.T, root string) demoAttributionSample {
	t.Helper()
	var sample demoAttributionSample
	readJSONFileForTest(t, filepath.Join(root, "examples", "demo", "attribution-precision-sample.json"), &sample)
	if sample.ObjectType != "relia.demo_attribution_precision_sample" {
		t.Fatalf("unexpected attribution sample object_type %q", sample.ObjectType)
	}
	return sample
}

func readDemoRedactionFixtures(t *testing.T, root string) demoRedactionFixtures {
	t.Helper()
	var fixtures demoRedactionFixtures
	readJSONFileForTest(t, filepath.Join(root, "examples", "demo", "redaction-fixtures", "expected-redacted-artifacts.json"), &fixtures)
	if fixtures.ObjectType != "relia.demo_redaction_fixture_expectations" {
		t.Fatalf("unexpected redaction fixture object_type %q", fixtures.ObjectType)
	}
	return fixtures
}

func readJSONFileForTest(t *testing.T, path string, target interface{}) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func demoPRURLs(t *testing.T, index demoPRIndex) map[int]string {
	t.Helper()
	urls := map[int]string{}
	for _, pr := range index.PRs {
		if pr.Number <= 0 || pr.URL == "" {
			t.Fatalf("invalid seeded PR entry: %#v", pr)
		}
		if previous, exists := urls[pr.Number]; exists {
			t.Fatalf("duplicate seeded PR #%d: %s and %s", pr.Number, previous, pr.URL)
		}
		urls[pr.Number] = pr.URL
	}
	return urls
}

func computeDemoStats(t *testing.T, outcomes []demoOutcomeFixture) (demoComputedStats, []demoRecurrencePair) {
	t.Helper()
	sort.Slice(outcomes, func(i, j int) bool {
		left, err := time.Parse(time.RFC3339, outcomes[i].RecordedAt)
		if err != nil {
			t.Fatalf("invalid recorded_at for %s: %v", outcomes[i].ExperienceID, err)
		}
		right, err := time.Parse(time.RFC3339, outcomes[j].RecordedAt)
		if err != nil {
			t.Fatalf("invalid recorded_at for %s: %v", outcomes[j].ExperienceID, err)
		}
		if left.Equal(right) {
			return outcomes[i].ExperienceID < outcomes[j].ExperienceID
		}
		return left.Before(right)
	})
	var stats demoComputedStats
	priorBySignature := map[string]demoOutcomeFixture{}
	var pairs []demoRecurrencePair
	for _, outcome := range outcomes {
		if outcome.ActorKind == "uncertain" {
			stats.attributionUncertainCount++
			continue
		}
		if outcome.OutcomeKind != "ci_failure" {
			continue
		}
		if outcome.FlakeDiscount > 0 {
			stats.flakeDiscountedCount++
			continue
		}
		if outcome.ActorKind != "agent" {
			continue
		}
		stats.agentFailureDenominator++
		if outcome.SignatureID == "" {
			t.Fatalf("agent failure %s missing signature_id", outcome.ExperienceID)
		}
		if prior, ok := priorBySignature[outcome.SignatureID]; ok {
			stats.confirmedRecurrenceCount++
			pairs = append(pairs, demoRecurrencePair{
				CurrentExperienceID: outcome.ExperienceID,
				PriorExperienceID:   prior.ExperienceID,
				CurrentPR:           outcome.PR,
				PriorPR:             prior.PR,
				Confidence:          "confirmed",
			})
		}
		priorBySignature[outcome.SignatureID] = outcome
	}
	return stats, pairs
}

func assertDemoPairsMatch(t *testing.T, got []demoRecurrencePair, want []demoRecurrencePair) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("confirmed recurrence pairs = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("confirmed recurrence pair %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func assertDemoReportCitationsResolve(t *testing.T, report demoBacktestReport, prURLs map[int]string) {
	t.Helper()
	for _, citation := range report.Citations {
		if prURLs[citation.PR] != citation.URL {
			t.Fatalf("citation for PR #%d resolves to %q, want %q", citation.PR, citation.URL, prURLs[citation.PR])
		}
	}
	requirePR := func(pr int, context string) {
		if _, ok := prURLs[pr]; !ok {
			t.Fatalf("%s references unknown seeded PR #%d", context, pr)
		}
	}
	for _, pair := range report.ConfirmedRecurrences {
		requirePR(pair.CurrentPR, "confirmed recurrence current")
		requirePR(pair.PriorPR, "confirmed recurrence prior")
	}
	for _, flake := range report.FlakeDiscounts {
		requirePR(flake.PR, "flake discount")
	}
	for _, rule := range report.RuleCandidates {
		for _, pr := range rule.EvidencePRs {
			requirePR(pr, "rule evidence")
		}
		if rule.HeldFixPR != 0 {
			requirePR(rule.HeldFixPR, "rule held fix")
		}
	}
}

func assertFlakeDiscountDraftsNoRule(t *testing.T, report demoBacktestReport) {
	t.Helper()
	flakySignatures := map[string]bool{}
	for _, flake := range report.FlakeDiscounts {
		if flake.FlakeDiscount <= 0 {
			t.Fatalf("flake discount for PR #%d is not positive", flake.PR)
		}
		if flake.DraftsRule {
			t.Fatalf("flake-discounted PR #%d drafts a rule", flake.PR)
		}
		flakySignatures[flake.SignatureID] = true
	}
	for _, rule := range report.RuleCandidates {
		if flakySignatures[rule.SignatureID] {
			t.Fatalf("flake-discounted signature %s produced rule %s", rule.SignatureID, rule.RuleID)
		}
	}
}

func assertDemoStaticReportTextMatchesJSON(t *testing.T, root string, report demoBacktestReport, prURLs map[int]string) {
	t.Helper()
	paths := []string{
		filepath.Join(root, "examples", "reports", "backtest-demo.html"),
		filepath.Join(root, "examples", "reports", "memory-page-demo.md"),
	}
	for _, path := range paths {
		contentBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(contentBytes)
		for _, token := range []string{
			report.Summary.HeadlineERRPercent,
			fmt.Sprintf("%d confirmed recurrences", report.Summary.ConfirmedRecurrenceCount),
			fmt.Sprintf("%d agent-attributed failures", report.Summary.AgentFailureDenominator),
			fmt.Sprintf("%d flake-discounted", report.Summary.FlakeDiscountedCount),
		} {
			if !strings.Contains(content, token) {
				t.Fatalf("%s missing reproducible report token %q", path, token)
			}
		}
		assertReportPRLinksResolve(t, content, prURLs)
	}
}

func assertReportPRLinksResolve(t *testing.T, content string, prURLs map[int]string) {
	t.Helper()
	linkPattern := regexp.MustCompile(`https://github\.com/Clyra-AI/relia-demo-seed/pull/([0-9]+)`)
	for _, match := range linkPattern.FindAllStringSubmatch(content, -1) {
		var pr int
		if _, err := fmt.Sscanf(match[1], "%d", &pr); err != nil {
			t.Fatalf("parse PR from report link %q: %v", match[0], err)
		}
		if prURLs[pr] != match[0] {
			t.Fatalf("report link %q does not resolve to seeded PR index", match[0])
		}
	}
}

func assertCustomerSafeDemoContent(t *testing.T, path string, content string) {
	t.Helper()
	for _, forbidden := range []string{
		"github_pat_",
		"ghp_",
		"gho_",
		"ghu_",
		"ghs_",
		"ghr_",
		"AKIA",
		"Bearer ",
		"/Users/",
		"customer.example",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("%s contains forbidden customer-unsafe token %q", path, forbidden)
		}
	}
	for _, pattern := range knownSecretPatterns {
		if pattern.MatchString(content) {
			t.Fatalf("%s contains a standard secret token shape", path)
		}
	}
}
