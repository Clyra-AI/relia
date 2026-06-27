package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
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
	Paths                 []string `json:"paths"`
	ActorKind             string   `json:"actor_kind"`
	AttributionMethod     string   `json:"attribution_method"`
	AttributionConfidence float64  `json:"attribution_confidence"`
	OutcomeKind           string   `json:"outcome_kind"`
	SignatureID           string   `json:"signature_id"`
	CheckName             string   `json:"check_name"`
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
	ConfirmedRecurrences  []demoRecurrencePair       `json:"confirmed_recurrences"`
	FlakeDiscounts        []demoFlakeDiscount        `json:"flake_discounts"`
	AttributionUncertain  []demoUncertainCase        `json:"attribution_uncertain"`
	RuleCandidates        []demoRuleCandidate        `json:"rule_candidates"`
	RuleLifecycleOutcomes []demoRuleLifecycleOutcome `json:"rule_lifecycle_outcomes"`
	Citations             []demoCitation             `json:"citations"`
	RedactionStatus       string                     `json:"redaction_status"`
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
	ExperienceID  string  `json:"experience_id"`
	SignatureID   string  `json:"signature_id"`
	FlakeDiscount float64 `json:"flake_discount"`
	DraftsRule    bool    `json:"drafts_rule"`
	SupportingPRs []int   `json:"supporting_prs"`
}

type demoUncertainCase struct {
	PR                    int     `json:"pr"`
	ExperienceID          string  `json:"experience_id"`
	OutcomeKind           string  `json:"outcome_kind"`
	AttributionMethod     string  `json:"attribution_method"`
	AttributionConfidence float64 `json:"attribution_confidence"`
	ExcludedFromERR       bool    `json:"excluded_from_err"`
	Reason                string  `json:"reason"`
}

type demoRuleCandidate struct {
	RuleID      string `json:"rule_id"`
	Kind        string `json:"kind"`
	SignatureID string `json:"signature_id"`
	EvidencePRs []int  `json:"evidence_prs"`
	HeldFixPR   int    `json:"held_fix_pr,omitempty"`
}

type demoRuleLifecycleOutcome struct {
	RuleID         string `json:"rule_id"`
	Status         string `json:"status"`
	SignatureID    string `json:"signature_id"`
	EvidencePRs    []int  `json:"evidence_prs"`
	NoLongerServed bool   `json:"no_longer_served"`
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

type demoFlakeDiscountFixtures struct {
	ObjectType    string                         `json:"object_type"`
	SchemaVersion string                         `json:"schema_version"`
	Cases         []demoFlakeDiscountFixtureCase `json:"cases"`
}

type demoFlakeDiscountFixtureCase struct {
	CaseID                  string   `json:"case_id"`
	PR                      int      `json:"pr"`
	ExperienceID            string   `json:"experience_id"`
	SignatureID             string   `json:"signature_id"`
	FlakeDiscount           float64  `json:"flake_discount"`
	ExpectedRuleDraft       bool     `json:"expected_rule_draft"`
	Citation                string   `json:"citation"`
	SupportingExperienceIDs []string `json:"supporting_experience_ids"`
}

type demoRedactionFixtures struct {
	ObjectType    string `json:"object_type"`
	SchemaVersion string `json:"schema_version"`
	Cases         []struct {
		CaseID                string   `json:"case_id"`
		StoresRawSecret       bool     `json:"stores_raw_secret"`
		ExpectedArtifactValue string   `json:"expected_artifact_value"`
		ArtifactPathsChecked  []string `json:"artifact_paths_checked"`
	} `json:"cases"`
}

type demoLifecycleFixtures struct {
	ObjectType      string                 `json:"object_type"`
	SchemaVersion   string                 `json:"schema_version"`
	SourceArtifacts []string               `json:"source_artifacts"`
	RedactionStatus string                 `json:"redaction_status"`
	Cases           []demoLifecycleCase    `json:"cases"`
	ServingSnapshot demoCompiledContext    `json:"serving_snapshot"`
	Metadata        map[string]interface{} `json:"metadata"`
}

type demoLifecycleCase struct {
	CaseID               string                  `json:"case_id"`
	Command              string                  `json:"command"`
	LifecycleOutcome     string                  `json:"lifecycle_outcome"`
	ExpectedStatus       string                  `json:"expected_status"`
	ServingEligible      bool                    `json:"serving_eligible"`
	TriggerExperienceIDs []string                `json:"trigger_experience_ids"`
	EvidenceRefs         []string                `json:"evidence_refs"`
	Citations            []demoLifecycleCitation `json:"citations"`
	Rule                 demoLifecycleRule       `json:"rule"`
}

type demoLifecycleCitation struct {
	PR           int    `json:"pr"`
	URL          string `json:"url"`
	ExperienceID string `json:"experience_id"`
	Role         string `json:"role"`
}

type demoLifecycleRule struct {
	ObjectType    string                    `json:"object_type"`
	SchemaVersion string                    `json:"schema_version"`
	ID            string                    `json:"id"`
	Kind          string                    `json:"kind"`
	Status        string                    `json:"status"`
	Statement     string                    `json:"statement"`
	Scope         demoLifecycleRuleScope    `json:"scope"`
	Confidence    float64                   `json:"confidence"`
	Evidence      demoLifecycleRuleEvidence `json:"evidence"`
	Provenance    []demoLifecycleProvenance `json:"provenance"`
	Review        demoLifecycleRuleReview   `json:"review"`
	Metadata      map[string]interface{}    `json:"metadata"`
}

type demoLifecycleRuleScope struct {
	Paths   []string `json:"paths"`
	Signals []string `json:"signals"`
}

type demoLifecycleRuleEvidence struct {
	Count          int      `json:"count"`
	Contradictions int      `json:"contradictions"`
	Experiences    []string `json:"experiences"`
}

type demoLifecycleProvenance struct {
	PR      int    `json:"pr"`
	Outcome string `json:"outcome"`
	URL     string `json:"url"`
}

type demoLifecycleRuleReview struct {
	Label           string `json:"label"`
	StatementOrigin string `json:"statement_origin"`
}

type demoCompiledContext struct {
	ObjectType    string                    `json:"object_type"`
	SchemaVersion string                    `json:"schema_version"`
	ContextID     string                    `json:"context_id"`
	Target        string                    `json:"target"`
	Rules         []demoCompiledContextRule `json:"rules"`
	Metadata      map[string]interface{}    `json:"metadata"`
}

type demoCompiledContextRule struct {
	RuleID        string `json:"rule_id"`
	Status        string `json:"status"`
	CitationCount int    `json:"citation_count"`
}

type demoAssessmentFixtures struct {
	ObjectType      string                 `json:"object_type"`
	SchemaVersion   string                 `json:"schema_version"`
	SourceArtifacts []string               `json:"source_artifacts"`
	RedactionStatus string                 `json:"redaction_status"`
	Cases           []demoAssessmentCase   `json:"cases"`
	Metadata        map[string]interface{} `json:"metadata"`
}

type demoAssessmentCase struct {
	CaseID               string                       `json:"case_id"`
	Command              string                       `json:"command"`
	InputDiff            string                       `json:"input_diff"`
	ExpectedRiskLevel    string                       `json:"expected_risk_level"`
	ExpectedTouchedPaths []string                     `json:"expected_touched_paths"`
	ExpectedMatches      []demoAssessmentExpectedRule `json:"expected_matches"`
	ExpectedCitations    []string                     `json:"expected_citations"`
	EvidenceRefs         []string                     `json:"evidence_refs"`
}

type demoAssessmentExpectedRule struct {
	RuleID     string  `json:"rule_id"`
	Confidence float64 `json:"confidence"`
}

type demoRiskAssessment struct {
	ObjectType    string                       `json:"object_type"`
	SchemaVersion string                       `json:"schema_version"`
	AssessmentID  string                       `json:"assessment_id"`
	RiskLevel     string                       `json:"risk_level"`
	Matches       []demoAssessmentExpectedRule `json:"matches"`
	Citations     []string                     `json:"citations"`
	Metadata      map[string]interface{}       `json:"metadata"`
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
	assertUncertainAttributionVisible(t, outcomes, report, prURLs)
	assertDemoReportCitationsResolve(t, report, prURLs)
	assertFlakeDiscountDraftsNoRule(t, report)
	assertFlakeDiscountFixturesBackedByRepeatedSeededFailures(t, root, outcomes, report, prURLs)
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

func TestDemoDistillReviewLifecycleFixtures(t *testing.T) {
	root := findRepoRootForTest(t)
	prURLs := demoPRURLs(t, readDemoPRIndex(t, root))
	outcomesByExperience := demoOutcomesByExperience(readDemoOutcomes(t, root))
	fixtures := readDemoLifecycleFixtures(t, root)

	assertLifecycleFixturePathsAreRepoRelative(t, root, fixtures.SourceArtifacts)
	assertCompiledContextSchemaShape(t, fixtures.ServingSnapshot)
	servingRuleIDs := map[string]bool{}
	for _, rule := range fixtures.ServingSnapshot.Rules {
		servingRuleIDs[rule.RuleID] = true
	}

	wantCaseIDs := map[string]bool{
		"planted-recurrence-drafts-avoid-rule":            false,
		"planted-contradiction-moves-rule-out-of-serving": false,
		"planted-path-deletion-moves-rule-out-of-serving": false,
	}
	for _, fixtureCase := range fixtures.Cases {
		if _, ok := wantCaseIDs[fixtureCase.CaseID]; !ok {
			t.Fatalf("unexpected lifecycle fixture case %s", fixtureCase.CaseID)
		}
		wantCaseIDs[fixtureCase.CaseID] = true
		assertLifecycleFixturePathsAreRepoRelative(t, root, fixtureCase.EvidenceRefs)
		assertLifecycleRuleSchemaShape(t, fixtureCase.Rule)
		assertLifecycleCitationsResolve(t, fixtureCase, prURLs, outcomesByExperience)
		if fixtureCase.ExpectedStatus != fixtureCase.Rule.Status {
			t.Fatalf("%s expected_status = %q, rule status = %q", fixtureCase.CaseID, fixtureCase.ExpectedStatus, fixtureCase.Rule.Status)
		}
		if !fixtureCase.ServingEligible && servingRuleIDs[fixtureCase.Rule.ID] {
			t.Fatalf("%s rule %s is not serving-eligible but appears in serving snapshot", fixtureCase.CaseID, fixtureCase.Rule.ID)
		}
		servingEligible, ok := fixtureCase.Rule.Metadata["serving_eligible"].(bool)
		if !ok || servingEligible != fixtureCase.ServingEligible {
			t.Fatalf("%s metadata serving_eligible = %#v, want %v", fixtureCase.CaseID, fixtureCase.Rule.Metadata["serving_eligible"], fixtureCase.ServingEligible)
		}

		switch fixtureCase.CaseID {
		case "planted-recurrence-drafts-avoid-rule":
			assertLifecycleRecurrenceDraft(t, fixtureCase, outcomesByExperience)
		case "planted-contradiction-moves-rule-out-of-serving":
			assertLifecycleContradictedRule(t, fixtureCase, outcomesByExperience)
		case "planted-path-deletion-moves-rule-out-of-serving":
			assertLifecycleStaleRule(t, fixtureCase, outcomesByExperience)
		}
	}
	for caseID, seen := range wantCaseIDs {
		if !seen {
			t.Fatalf("missing lifecycle fixture case %s", caseID)
		}
	}
	assertLifecycleOutcomesVisibleOnMemoryPage(t, root, fixtures, prURLs)
}

func TestDemoAssessmentFixturesDriveAssessCommand(t *testing.T) {
	root := findRepoRootForTest(t)
	prURLs := demoPRURLs(t, readDemoPRIndex(t, root))
	fixtures := readDemoAssessmentFixtures(t, root)

	assertAssessmentFixturePathsAreRepoRelative(t, root, fixtures.SourceArtifacts)
	wantCaseIDs := map[string]bool{
		"planted-pattern-match-high": false,
		"unknown-path-no-coverage":   false,
	}
	for _, fixtureCase := range fixtures.Cases {
		if _, ok := wantCaseIDs[fixtureCase.CaseID]; !ok {
			t.Fatalf("unexpected assessment fixture case %s", fixtureCase.CaseID)
		}
		wantCaseIDs[fixtureCase.CaseID] = true
		assertAssessmentFixturePathsAreRepoRelative(t, root, append([]string{fixtureCase.InputDiff}, fixtureCase.EvidenceRefs...))
		assertAssessmentDiffFixtureMatchesDeclaredPaths(t, root, fixtureCase)
		assertAssessmentCitationsResolve(t, fixtureCase, prURLs)
		assertAssessmentCommandMatchesFixture(t, fixtureCase)
	}
	for caseID, seen := range wantCaseIDs {
		if !seen {
			t.Fatalf("missing assessment fixture case %s", caseID)
		}
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
		assertDemoRedactionFixtureExercisesPipeline(t, testCase.CaseID, testCase.ExpectedArtifactValue)
		if len(testCase.ArtifactPathsChecked) == 0 {
			t.Fatalf("redaction fixture %s does not list artifact paths checked", testCase.CaseID)
		}
		for _, relPath := range testCase.ArtifactPathsChecked {
			content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
			if err != nil {
				t.Fatalf("read redaction artifact %s: %v", relPath, err)
			}
			if !strings.Contains(string(content), testCase.ExpectedArtifactValue) {
				t.Fatalf("redaction fixture %s expected %s in %s", testCase.CaseID, testCase.ExpectedArtifactValue, relPath)
			}
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

func assertDemoRedactionFixtureExercisesPipeline(t *testing.T, caseID string, expectedArtifactValue string) {
	t.Helper()
	const syntheticToken = "ghp_1234567890abcdef1234567890abcdef123456"
	rawEvent := map[string]any{
		"message": "demo failure log Bearer " + syntheticToken,
		"metadata": map[string]any{
			"client_secret": "demo-client-secret",
		},
	}
	redacted, commandErr := redactForPersistence(rawEvent, "demo-redaction-fixture:"+caseID)
	if commandErr != nil {
		t.Fatalf("redaction fixture %s failed redaction pipeline: %v", caseID, commandErr)
	}
	encoded, err := json.Marshal(redacted)
	if err != nil {
		t.Fatalf("encode redacted fixture %s: %v", caseID, err)
	}
	if strings.Contains(string(encoded), syntheticToken) || strings.Contains(string(encoded), "demo-client-secret") {
		t.Fatalf("redaction fixture %s preserved raw secret material: %s", caseID, encoded)
	}
	if !strings.Contains(string(encoded), expectedArtifactValue) {
		t.Fatalf("redaction fixture %s did not produce expected placeholder %s: %s", caseID, expectedArtifactValue, encoded)
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

func readDemoFlakeDiscountFixtures(t *testing.T, root string) demoFlakeDiscountFixtures {
	t.Helper()
	var fixtures demoFlakeDiscountFixtures
	readJSONFileForTest(t, filepath.Join(root, "examples", "demo", "flake-discount-fixtures.json"), &fixtures)
	if fixtures.ObjectType != "relia.demo_flake_discount_fixtures" {
		t.Fatalf("unexpected flake discount fixture object_type %q", fixtures.ObjectType)
	}
	return fixtures
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

func readDemoLifecycleFixtures(t *testing.T, root string) demoLifecycleFixtures {
	t.Helper()
	path := filepath.Join(root, "examples", "demo", "distill-review-lifecycle-fixtures.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures demoLifecycleFixtures
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixtures); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if fixtures.ObjectType != "relia.demo_distill_review_lifecycle_fixtures" {
		t.Fatalf("unexpected lifecycle fixture object_type %q", fixtures.ObjectType)
	}
	if fixtures.SchemaVersion != commandSchemaVersion {
		t.Fatalf("lifecycle fixture schema_version = %q", fixtures.SchemaVersion)
	}
	if fixtures.RedactionStatus != "customer_safe" {
		t.Fatalf("lifecycle fixture redaction_status = %q", fixtures.RedactionStatus)
	}
	if customerSafe, ok := fixtures.Metadata["customer_safe"].(bool); !ok || !customerSafe {
		t.Fatalf("lifecycle fixture metadata.customer_safe = %#v", fixtures.Metadata["customer_safe"])
	}
	return fixtures
}

func readDemoAssessmentFixtures(t *testing.T, root string) demoAssessmentFixtures {
	t.Helper()
	path := filepath.Join(root, "examples", "demo", "assessment-fixtures", "assessment-fixtures.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures demoAssessmentFixtures
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixtures); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if fixtures.ObjectType != "relia.demo_assessment_fixtures" {
		t.Fatalf("unexpected assessment fixture object_type %q", fixtures.ObjectType)
	}
	if fixtures.SchemaVersion != commandSchemaVersion {
		t.Fatalf("assessment fixture schema_version = %q", fixtures.SchemaVersion)
	}
	if fixtures.RedactionStatus != "customer_safe" {
		t.Fatalf("assessment fixture redaction_status = %q", fixtures.RedactionStatus)
	}
	if customerSafe, ok := fixtures.Metadata["customer_safe"].(bool); !ok || !customerSafe {
		t.Fatalf("assessment fixture metadata.customer_safe = %#v", fixtures.Metadata["customer_safe"])
	}
	if repoRelative, ok := fixtures.Metadata["repo_relative_paths_only"].(bool); !ok || !repoRelative {
		t.Fatalf("assessment fixture metadata.repo_relative_paths_only = %#v", fixtures.Metadata["repo_relative_paths_only"])
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

func demoOutcomesByExperience(outcomes []demoOutcomeFixture) map[string]demoOutcomeFixture {
	byExperience := map[string]demoOutcomeFixture{}
	for _, outcome := range outcomes {
		byExperience[outcome.ExperienceID] = outcome
	}
	return byExperience
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
		if outcome.ActorKind != "agent" {
			continue
		}
		stats.agentFailureDenominator++
		if outcome.FlakeDiscount > 0 {
			stats.flakeDiscountedCount++
			continue
		}
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
		for _, supportingPR := range flake.SupportingPRs {
			requirePR(supportingPR, "flake discount support")
		}
	}
	for _, uncertain := range report.AttributionUncertain {
		requirePR(uncertain.PR, "uncertain attribution")
	}
	for _, rule := range report.RuleCandidates {
		for _, pr := range rule.EvidencePRs {
			requirePR(pr, "rule evidence")
		}
		if rule.HeldFixPR != 0 {
			requirePR(rule.HeldFixPR, "rule held fix")
		}
	}
	for _, outcome := range report.RuleLifecycleOutcomes {
		for _, pr := range outcome.EvidencePRs {
			requirePR(pr, "rule lifecycle outcome evidence")
		}
	}
}

func assertLifecycleFixturePathsAreRepoRelative(t *testing.T, root string, refs []string) {
	t.Helper()
	if len(refs) == 0 {
		t.Fatal("lifecycle fixture does not cite repo-relative evidence refs")
	}
	for _, ref := range refs {
		assertRepoRelativeFixturePath(t, ref)
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(ref))); err != nil {
			t.Fatalf("lifecycle fixture evidence ref %s is not readable: %v", ref, err)
		}
	}
}

func assertAssessmentFixturePathsAreRepoRelative(t *testing.T, root string, refs []string) {
	t.Helper()
	if len(refs) == 0 {
		t.Fatal("assessment fixture does not cite repo-relative evidence refs")
	}
	for _, ref := range refs {
		assertRepoRelativeFixturePath(t, ref)
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(ref))); err != nil {
			t.Fatalf("assessment fixture evidence ref %s is not readable: %v", ref, err)
		}
	}
}

func assertAssessmentDiffFixtureMatchesDeclaredPaths(t *testing.T, root string, fixtureCase demoAssessmentCase) {
	t.Helper()
	assertRepoRelativeFixturePath(t, fixtureCase.InputDiff)
	diffPath := filepath.Join(root, filepath.FromSlash(fixtureCase.InputDiff))
	content, err := os.ReadFile(diffPath)
	if err != nil {
		t.Fatalf("%s input_diff %s is not readable: %v", fixtureCase.CaseID, fixtureCase.InputDiff, err)
	}
	parsed, commandErr := parseUnifiedDiffTouchedPaths(content, fixtureCase.InputDiff)
	if commandErr != nil {
		t.Fatalf("%s input_diff did not parse as a repo-relative diff: %v", fixtureCase.CaseID, commandErr)
	}
	if fmt.Sprint(parsed) != fmt.Sprint(fixtureCase.ExpectedTouchedPaths) {
		t.Fatalf("%s parsed touched_paths = %#v, want declared expected_touched_paths %#v", fixtureCase.CaseID, parsed, fixtureCase.ExpectedTouchedPaths)
	}
	if output, err := exec.Command("git", "-C", root, "apply", "--check", fixtureCase.InputDiff).CombinedOutput(); err != nil {
		t.Fatalf("%s input_diff is not replayable with git apply --check: %v\n%s", fixtureCase.CaseID, err, string(output))
	}
	for _, touchedPath := range fixtureCase.ExpectedTouchedPaths {
		assertRepoRelativeFixturePath(t, touchedPath)
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(touchedPath))); err != nil {
			t.Fatalf("%s expected touched path %s is not backed by a fixture file: %v", fixtureCase.CaseID, touchedPath, err)
		}
	}
}

func assertRepoRelativeFixturePath(t *testing.T, ref string) {
	t.Helper()
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(ref)))
	if strings.TrimSpace(ref) == "" || filepath.IsAbs(ref) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		t.Fatalf("fixture path is not repo-relative: %q", ref)
	}
	if strings.Contains(ref, "\\") {
		t.Fatalf("fixture path must use slash-separated repo refs: %q", ref)
	}
}

func assertCompiledContextSchemaShape(t *testing.T, snapshot demoCompiledContext) {
	t.Helper()
	if snapshot.ObjectType != "relia.compiled_context" {
		t.Fatalf("serving snapshot object_type = %q", snapshot.ObjectType)
	}
	if snapshot.SchemaVersion != commandSchemaVersion {
		t.Fatalf("serving snapshot schema_version = %q", snapshot.SchemaVersion)
	}
	if strings.TrimSpace(snapshot.ContextID) == "" {
		t.Fatal("serving snapshot missing context_id")
	}
	if snapshot.Target != "AGENTS.md" && snapshot.Target != "CLAUDE.md" {
		t.Fatalf("serving snapshot target = %q", snapshot.Target)
	}
	for _, rule := range snapshot.Rules {
		if strings.TrimSpace(rule.RuleID) == "" {
			t.Fatal("serving snapshot rule missing rule_id")
		}
		if rule.Status != "active" {
			t.Fatalf("serving snapshot rule %s status = %q, want active", rule.RuleID, rule.Status)
		}
		if rule.CitationCount < 1 {
			t.Fatalf("serving snapshot rule %s citation_count = %d", rule.RuleID, rule.CitationCount)
		}
	}
}

func assertLifecycleRuleSchemaShape(t *testing.T, rule demoLifecycleRule) {
	t.Helper()
	if rule.ObjectType != "relia.memory_rule" {
		t.Fatalf("lifecycle rule %s object_type = %q", rule.ID, rule.ObjectType)
	}
	if rule.SchemaVersion != commandSchemaVersion {
		t.Fatalf("lifecycle rule %s schema_version = %q", rule.ID, rule.SchemaVersion)
	}
	if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.Statement) == "" {
		t.Fatalf("lifecycle rule has empty id or statement: %#v", rule)
	}
	if rule.Kind != "avoid" && rule.Kind != "playbook" {
		t.Fatalf("lifecycle rule %s kind = %q", rule.ID, rule.Kind)
	}
	switch rule.Status {
	case "candidate", "active", "stale", "contradicted", "retired":
	default:
		t.Fatalf("lifecycle rule %s status = %q", rule.ID, rule.Status)
	}
	if rule.Confidence < 0 || rule.Confidence > 1 {
		t.Fatalf("lifecycle rule %s confidence = %.3f", rule.ID, rule.Confidence)
	}
	if len(rule.Scope.Paths) == 0 && len(rule.Scope.Signals) == 0 {
		t.Fatalf("lifecycle rule %s has no concrete scope", rule.ID)
	}
	for _, ref := range rule.Scope.Paths {
		assertRepoRelativeFixturePath(t, ref)
	}
	if rule.Evidence.Count != len(rule.Evidence.Experiences) || rule.Evidence.Count < 1 {
		t.Fatalf("lifecycle rule %s evidence count = %d, experiences = %#v", rule.ID, rule.Evidence.Count, rule.Evidence.Experiences)
	}
	if rule.Evidence.Contradictions < 0 {
		t.Fatalf("lifecycle rule %s has negative contradictions", rule.ID)
	}
	if len(rule.Provenance) != rule.Evidence.Count {
		t.Fatalf("lifecycle rule %s provenance entries = %d, evidence count = %d", rule.ID, len(rule.Provenance), rule.Evidence.Count)
	}
	switch rule.Review.Label {
	case "accepted", "suggested", "needs_user_input":
	default:
		t.Fatalf("lifecycle rule %s review.label = %q", rule.ID, rule.Review.Label)
	}
	if rule.Status == "active" && rule.Review.Label != "accepted" {
		t.Fatalf("active lifecycle rule %s review.label = %q", rule.ID, rule.Review.Label)
	}
	switch rule.Review.StatementOrigin {
	case "llm_drafted", "cluster_summary", "human_authored":
	default:
		t.Fatalf("lifecycle rule %s review.statement_origin = %q", rule.ID, rule.Review.StatementOrigin)
	}
}

func assertLifecycleCitationsResolve(t *testing.T, fixtureCase demoLifecycleCase, prURLs map[int]string, outcomesByExperience map[string]demoOutcomeFixture) {
	t.Helper()
	citationsByPR := map[int]demoLifecycleCitation{}
	for _, citation := range fixtureCase.Citations {
		if prURLs[citation.PR] != citation.URL {
			t.Fatalf("%s citation for PR #%d resolves to %q, want %q", fixtureCase.CaseID, citation.PR, citation.URL, prURLs[citation.PR])
		}
		if strings.TrimSpace(citation.Role) == "" {
			t.Fatalf("%s citation for PR #%d missing role", fixtureCase.CaseID, citation.PR)
		}
		outcome, ok := outcomesByExperience[citation.ExperienceID]
		if !ok {
			t.Fatalf("%s citation references unknown experience %s", fixtureCase.CaseID, citation.ExperienceID)
		}
		if outcome.PR != citation.PR {
			t.Fatalf("%s citation %s PR = %d, want %d", fixtureCase.CaseID, citation.ExperienceID, citation.PR, outcome.PR)
		}
		citationsByPR[citation.PR] = citation
	}
	for _, provenance := range fixtureCase.Rule.Provenance {
		citation, ok := citationsByPR[provenance.PR]
		if !ok {
			t.Fatalf("%s provenance PR #%d has no citation", fixtureCase.CaseID, provenance.PR)
		}
		if provenance.URL != citation.URL || prURLs[provenance.PR] != provenance.URL {
			t.Fatalf("%s provenance for PR #%d has unresolved URL %q", fixtureCase.CaseID, provenance.PR, provenance.URL)
		}
		outcome := outcomesByExperience[citation.ExperienceID]
		if outcome.OutcomeKind != provenance.Outcome {
			t.Fatalf("%s provenance PR #%d outcome = %q, want outcome stream kind %q", fixtureCase.CaseID, provenance.PR, provenance.Outcome, outcome.OutcomeKind)
		}
	}
	for _, experienceID := range fixtureCase.TriggerExperienceIDs {
		if _, ok := outcomesByExperience[experienceID]; !ok {
			t.Fatalf("%s trigger references unknown experience %s", fixtureCase.CaseID, experienceID)
		}
	}
}

func assertAssessmentCitationsResolve(t *testing.T, fixtureCase demoAssessmentCase, prURLs map[int]string) {
	t.Helper()
	if fixtureCase.ExpectedRiskLevel == "match_high" && len(fixtureCase.ExpectedCitations) == 0 {
		t.Fatalf("%s expected match_high without citations", fixtureCase.CaseID)
	}
	for _, citation := range fixtureCase.ExpectedCitations {
		found := false
		for _, url := range prURLs {
			if citation == url {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s citation %q does not resolve through seeded PR index", fixtureCase.CaseID, citation)
		}
	}
}

func assertAssessmentCommandMatchesFixture(t *testing.T, fixtureCase demoAssessmentCase) {
	t.Helper()
	stdout, stderr, code := runForTest(t, []string{"--json", "assess", "--input", fixtureCase.InputDiff}, false)
	if code != ExitSuccess {
		t.Fatalf("%s exit code = %d, stderr = %q, stdout = %q", fixtureCase.CaseID, code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "assess" || result.Status != "pass" {
		t.Fatalf("%s command result = %#v", fixtureCase.CaseID, result)
	}
	assessment := decodeAssessmentFromResult(t, result)
	assertAssessmentMatchesExpected(t, fixtureCase, assessment)

	repeatedStdout, repeatedStderr, repeatedCode := runForTest(t, []string{"--json", "assess", "--input", fixtureCase.InputDiff}, false)
	if repeatedCode != ExitSuccess {
		t.Fatalf("%s repeated exit code = %d, stderr = %q, stdout = %q", fixtureCase.CaseID, repeatedCode, repeatedStderr, repeatedStdout)
	}
	repeatedAssessment := decodeAssessmentFromResult(t, decodeResult(t, repeatedStdout))
	if assessment.AssessmentID != repeatedAssessment.AssessmentID {
		t.Fatalf("%s assessment_id changed across repeated runs: %q then %q", fixtureCase.CaseID, assessment.AssessmentID, repeatedAssessment.AssessmentID)
	}
}

func decodeAssessmentFromResult(t *testing.T, result CommandResult) demoRiskAssessment {
	t.Helper()
	encoded, err := json.Marshal(result.Data["assessment"])
	if err != nil {
		t.Fatalf("encode nested assessment: %v", err)
	}
	var assessment demoRiskAssessment
	if err := json.Unmarshal(encoded, &assessment); err != nil {
		t.Fatalf("decode nested assessment from %s: %v", encoded, err)
	}
	if assessment.ObjectType != "relia.risk_assessment" {
		t.Fatalf("assessment object_type = %q", assessment.ObjectType)
	}
	if assessment.SchemaVersion != commandSchemaVersion {
		t.Fatalf("assessment schema_version = %q", assessment.SchemaVersion)
	}
	if strings.TrimSpace(assessment.AssessmentID) == "" {
		t.Fatal("assessment missing assessment_id")
	}
	return assessment
}

func assertAssessmentMatchesExpected(t *testing.T, fixtureCase demoAssessmentCase, assessment demoRiskAssessment) {
	t.Helper()
	if assessment.RiskLevel != fixtureCase.ExpectedRiskLevel {
		t.Fatalf("%s risk_level = %q, want %q", fixtureCase.CaseID, assessment.RiskLevel, fixtureCase.ExpectedRiskLevel)
	}
	if fmt.Sprint(assessment.Matches) != fmt.Sprint(fixtureCase.ExpectedMatches) {
		t.Fatalf("%s matches = %#v, want %#v", fixtureCase.CaseID, assessment.Matches, fixtureCase.ExpectedMatches)
	}
	if fmt.Sprint(assessment.Citations) != fmt.Sprint(fixtureCase.ExpectedCitations) {
		t.Fatalf("%s citations = %#v, want %#v", fixtureCase.CaseID, assessment.Citations, fixtureCase.ExpectedCitations)
	}
	inputPath, _ := assessment.Metadata["input_path"].(string)
	if inputPath != fixtureCase.InputDiff {
		t.Fatalf("%s assessment input_path = %q, want %q", fixtureCase.CaseID, inputPath, fixtureCase.InputDiff)
	}
	touched := stringsFromInterfaceSlice(t, assessment.Metadata["touched_paths"])
	if fmt.Sprint(touched) != fmt.Sprint(fixtureCase.ExpectedTouchedPaths) {
		t.Fatalf("%s touched_paths = %#v, want %#v", fixtureCase.CaseID, touched, fixtureCase.ExpectedTouchedPaths)
	}
	repoRelative, ok := assessment.Metadata["repo_relative_paths_only"].(bool)
	if !ok || !repoRelative {
		t.Fatalf("%s metadata.repo_relative_paths_only = %#v", fixtureCase.CaseID, assessment.Metadata["repo_relative_paths_only"])
	}
}

func assertLifecycleRecurrenceDraft(t *testing.T, fixtureCase demoLifecycleCase, outcomesByExperience map[string]demoOutcomeFixture) {
	t.Helper()
	if fixtureCase.LifecycleOutcome != "drafted" || fixtureCase.Rule.Kind != "avoid" || fixtureCase.Rule.Status != "candidate" {
		t.Fatalf("recurrence fixture has wrong lifecycle shape: %#v", fixtureCase)
	}
	if fixtureCase.Rule.Review.Label != "suggested" || fixtureCase.Rule.Review.StatementOrigin != "cluster_summary" {
		t.Fatalf("recurrence fixture review = %#v", fixtureCase.Rule.Review)
	}
	signatureIDs := map[string]bool{}
	for _, experienceID := range fixtureCase.Rule.Evidence.Experiences {
		outcome := outcomesByExperience[experienceID]
		if outcome.ActorKind != "agent" || outcome.OutcomeKind != "ci_failure" || outcome.FlakeDiscount != 0 {
			t.Fatalf("recurrence evidence %s is not an undiscounted agent failure: %#v", experienceID, outcome)
		}
		signatureIDs[outcome.SignatureID] = true
	}
	if len(signatureIDs) != 1 || !signatureIDs["sig_time_freeze"] {
		t.Fatalf("recurrence evidence does not form the planted sig_time_freeze cluster: %#v", signatureIDs)
	}
	if fixtureCase.Rule.Evidence.Count < 3 {
		t.Fatalf("recurrence evidence count = %d, want at least 3", fixtureCase.Rule.Evidence.Count)
	}
}

func assertLifecycleContradictedRule(t *testing.T, fixtureCase demoLifecycleCase, outcomesByExperience map[string]demoOutcomeFixture) {
	t.Helper()
	if fixtureCase.LifecycleOutcome != "contradicted" || fixtureCase.Rule.Status != "contradicted" {
		t.Fatalf("contradiction fixture has wrong lifecycle shape: %#v", fixtureCase)
	}
	if fixtureCase.Rule.Evidence.Contradictions < 2 {
		t.Fatalf("contradiction fixture contradictions = %d, want at least 2", fixtureCase.Rule.Evidence.Contradictions)
	}
	var cleanContradictions int
	for _, citation := range fixtureCase.Citations {
		outcome := outcomesByExperience[citation.ExperienceID]
		if citation.Role == "contradiction" {
			if outcome.OutcomeKind != "merged_clean" || outcome.SignatureID != "sig_schema_snapshot_blind_regen" {
				t.Fatalf("contradiction citation is not a clean merge of the planted signature: %#v", outcome)
			}
			cleanContradictions++
		}
	}
	if cleanContradictions < 2 {
		t.Fatalf("contradiction fixture has %d clean contradiction citations, want at least 2", cleanContradictions)
	}
}

func assertLifecycleStaleRule(t *testing.T, fixtureCase demoLifecycleCase, outcomesByExperience map[string]demoOutcomeFixture) {
	t.Helper()
	if fixtureCase.LifecycleOutcome != "stale" || fixtureCase.Rule.Status != "stale" {
		t.Fatalf("stale fixture has wrong lifecycle shape: %#v", fixtureCase)
	}
	stalePath, ok := fixtureCase.Rule.Metadata["stale_path"].(string)
	if !ok || strings.TrimSpace(stalePath) == "" {
		t.Fatalf("stale fixture missing metadata.stale_path: %#v", fixtureCase.Rule.Metadata)
	}
	assertRepoRelativeFixturePath(t, stalePath)
	if !containsStringValue(fixtureCase.Rule.Scope.Paths, stalePath) {
		t.Fatalf("stale fixture stale_path %q not present in rule scope %#v", stalePath, fixtureCase.Rule.Scope.Paths)
	}
	deletionPR, ok := fixtureCase.Rule.Metadata["deletion_pr"].(float64)
	if !ok || int(deletionPR) != 293 {
		t.Fatalf("stale fixture deletion_pr = %#v, want 293", fixtureCase.Rule.Metadata["deletion_pr"])
	}
	var foundDeletionCitation bool
	originalScopePaths := map[string]bool{}
	for _, citation := range fixtureCase.Citations {
		outcome := outcomesByExperience[citation.ExperienceID]
		switch citation.Role {
		case "original_failure":
			for _, path := range outcome.Paths {
				originalScopePaths[path] = true
			}
		case "scope_deleted":
			if citation.PR != 293 || outcome.OutcomeKind != "merged_clean" || outcome.SignatureID != "sig_time_freeze" {
				t.Fatalf("stale deletion citation has wrong outcome: %#v", outcome)
			}
			for _, scopedPath := range fixtureCase.Rule.Scope.Paths {
				if !containsStringValue(outcome.Paths, scopedPath) {
					t.Fatalf("stale deletion citation %s does not delete scoped path %q; deletion paths = %#v", citation.ExperienceID, scopedPath, outcome.Paths)
				}
			}
			foundDeletionCitation = true
		default:
			continue
		}
	}
	if !foundDeletionCitation {
		t.Fatal("stale fixture missing scope_deleted citation")
	}
	for _, scopedPath := range fixtureCase.Rule.Scope.Paths {
		if !originalScopePaths[scopedPath] {
			t.Fatalf("stale fixture scope path %q is not backed by original failure evidence %#v", scopedPath, originalScopePaths)
		}
	}
	if len(fixtureCase.Rule.Scope.Paths) != len(originalScopePaths) {
		t.Fatalf("stale fixture scope %#v narrowed or expanded original evidence scope %#v", fixtureCase.Rule.Scope.Paths, originalScopePaths)
	}
}

func assertLifecycleOutcomesVisibleOnMemoryPage(t *testing.T, root string, fixtures demoLifecycleFixtures, prURLs map[int]string) {
	t.Helper()
	var backtestReport demoBacktestReport
	readJSONFileForTest(t, filepath.Join(root, "examples", "reports", "backtest-demo.json"), &backtestReport)
	reportPaths := []string{
		filepath.Join("examples", "reports", "memory-page-demo.md"),
		filepath.Join("examples", "reports", "backtest-demo.html"),
	}
	for _, reportPath := range reportPaths {
		contentBytes, err := os.ReadFile(filepath.Join(root, reportPath))
		if err != nil {
			t.Fatal(err)
		}
		content := string(contentBytes)
		for _, fixtureCase := range fixtures.Cases {
			switch fixtureCase.Rule.Status {
			case "contradicted", "stale":
			default:
				continue
			}
			if !strings.Contains(content, fixtureCase.Rule.ID) {
				t.Fatalf("%s missing lifecycle rule %s", reportPath, fixtureCase.Rule.ID)
			}
			if !strings.Contains(content, "No longer served.") {
				t.Fatalf("%s does not mark lifecycle outcomes as out of serving", reportPath)
			}
			candidateHeading := fmt.Sprintf("## Candidate: %s", fixtureCase.Rule.ID)
			if strings.Contains(content, candidateHeading) {
				t.Fatalf("%s renders non-serving lifecycle rule %s as a candidate", reportPath, fixtureCase.Rule.ID)
			}
			htmlCandidate := fmt.Sprintf("<h2>Memory candidates</h2>")
			if strings.Contains(content, htmlCandidate) && strings.Contains(content, fixtureCase.Rule.ID) {
				t.Fatalf("%s renders non-serving lifecycle rule %s in Memory candidates", reportPath, fixtureCase.Rule.ID)
			}
			assertBacktestReportLifecycleOutcome(t, backtestReport, fixtureCase)
			for _, citation := range fixtureCase.Citations {
				if !strings.Contains(content, prURLs[citation.PR]) {
					t.Fatalf("%s missing lifecycle citation %s for %s", reportPath, prURLs[citation.PR], fixtureCase.CaseID)
				}
			}
		}
	}
}

func assertBacktestReportLifecycleOutcome(t *testing.T, report demoBacktestReport, fixtureCase demoLifecycleCase) {
	t.Helper()
	for _, candidate := range report.RuleCandidates {
		if candidate.RuleID == fixtureCase.Rule.ID {
			t.Fatalf("backtest JSON renders non-serving lifecycle rule %s as active candidate", fixtureCase.Rule.ID)
		}
	}
	for _, outcome := range report.RuleLifecycleOutcomes {
		if outcome.RuleID != fixtureCase.Rule.ID {
			continue
		}
		if outcome.Status != fixtureCase.Rule.Status || !outcome.NoLongerServed {
			t.Fatalf("backtest JSON lifecycle outcome for %s = %#v, want status %s and no_longer_served", fixtureCase.Rule.ID, outcome, fixtureCase.Rule.Status)
		}
		for _, citation := range fixtureCase.Citations {
			if !containsIntValue(outcome.EvidencePRs, citation.PR) {
				t.Fatalf("backtest JSON lifecycle outcome %s missing citation PR #%d", fixtureCase.Rule.ID, citation.PR)
			}
		}
		return
	}
	t.Fatalf("backtest JSON missing lifecycle outcome for %s", fixtureCase.Rule.ID)
}

func containsIntValue(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringsFromInterfaceSlice(t *testing.T, value interface{}) []string {
	t.Helper()
	items, ok := value.([]interface{})
	if !ok {
		t.Fatalf("value is not a JSON string array: %#v", value)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("array item is not a string: %#v", item)
		}
		result = append(result, text)
	}
	return result
}

func assertUncertainAttributionVisible(t *testing.T, outcomes []demoOutcomeFixture, report demoBacktestReport, prURLs map[int]string) {
	t.Helper()
	wantByExperience := map[string]demoOutcomeFixture{}
	for _, outcome := range outcomes {
		if outcome.ActorKind == "uncertain" {
			wantByExperience[outcome.ExperienceID] = outcome
		}
	}
	if len(report.AttributionUncertain) != len(wantByExperience) {
		t.Fatalf("attribution_uncertain entries = %d, want %d", len(report.AttributionUncertain), len(wantByExperience))
	}
	for _, item := range report.AttributionUncertain {
		outcome, ok := wantByExperience[item.ExperienceID]
		if !ok {
			t.Fatalf("attribution_uncertain references unknown or non-uncertain experience %s", item.ExperienceID)
		}
		if item.PR != outcome.PR || prURLs[item.PR] == "" {
			t.Fatalf("attribution_uncertain %s PR = %d, want seeded PR %d", item.ExperienceID, item.PR, outcome.PR)
		}
		if item.OutcomeKind != outcome.OutcomeKind || item.AttributionMethod != "uncertain" || item.AttributionConfidence != 0 || !item.ExcludedFromERR {
			t.Fatalf("attribution_uncertain %s does not preserve fail-closed attribution details: %#v", item.ExperienceID, item)
		}
		if strings.TrimSpace(item.Reason) == "" {
			t.Fatalf("attribution_uncertain %s missing reason", item.ExperienceID)
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

func assertFlakeDiscountFixturesBackedByRepeatedSeededFailures(t *testing.T, root string, outcomes []demoOutcomeFixture, report demoBacktestReport, prURLs map[int]string) {
	t.Helper()
	fixtures := readDemoFlakeDiscountFixtures(t, root)
	casesByPR := map[int]demoFlakeDiscountFixtureCase{}
	for _, fixtureCase := range fixtures.Cases {
		if fixtureCase.ExpectedRuleDraft {
			t.Fatalf("flake fixture %s expects a rule draft", fixtureCase.CaseID)
		}
		if prURLs[fixtureCase.PR] != fixtureCase.Citation {
			t.Fatalf("flake fixture %s citation = %q, want %q", fixtureCase.CaseID, fixtureCase.Citation, prURLs[fixtureCase.PR])
		}
		if len(fixtureCase.SupportingExperienceIDs) < 2 {
			t.Fatalf("flake fixture %s has %d supporting experiences, want at least 2", fixtureCase.CaseID, len(fixtureCase.SupportingExperienceIDs))
		}
		casesByPR[fixtureCase.PR] = fixtureCase
	}

	outcomesByExperience := map[string]demoOutcomeFixture{}
	for _, outcome := range outcomes {
		outcomesByExperience[outcome.ExperienceID] = outcome
	}
	for _, flake := range report.FlakeDiscounts {
		fixtureCase, ok := casesByPR[flake.PR]
		if !ok {
			t.Fatalf("report flake discount for PR #%d has no flake-discount fixture case", flake.PR)
		}
		if len(flake.SupportingPRs) < 2 {
			t.Fatalf("report flake discount for PR #%d has %d supporting PRs, want at least 2", flake.PR, len(flake.SupportingPRs))
		}
		primary, ok := outcomesByExperience[fixtureCase.ExperienceID]
		if !ok {
			t.Fatalf("flake fixture %s references unknown experience %s", fixtureCase.CaseID, fixtureCase.ExperienceID)
		}
		if primary.PR != fixtureCase.PR || primary.SignatureID != fixtureCase.SignatureID {
			t.Fatalf("flake fixture %s does not match primary outcome: %#v", fixtureCase.CaseID, primary)
		}
		if primary.FlakeDiscount != fixtureCase.FlakeDiscount || primary.FlakeDiscount <= 0 {
			t.Fatalf("flake fixture %s discount = %.3f, primary outcome discount = %.3f", fixtureCase.CaseID, fixtureCase.FlakeDiscount, primary.FlakeDiscount)
		}
		supportingPRs := map[int]bool{}
		for _, supportingPR := range flake.SupportingPRs {
			supportingPRs[supportingPR] = true
		}
		occurrences := []demoOutcomeFixture{primary}
		pathFingerprints := map[string]bool{demoPathFingerprint(primary.Paths): true}
		for _, supportID := range fixtureCase.SupportingExperienceIDs {
			support, ok := outcomesByExperience[supportID]
			if !ok {
				t.Fatalf("flake fixture %s references unknown support experience %s", fixtureCase.CaseID, supportID)
			}
			if !supportingPRs[support.PR] {
				t.Fatalf("flake fixture %s support PR #%d is missing from report support PRs", fixtureCase.CaseID, support.PR)
			}
			if support.SignatureID != primary.SignatureID || support.CheckName != primary.CheckName || support.SignatureKey != primary.SignatureKey {
				t.Fatalf("flake fixture %s support %s is not the same failing check/signature as primary %s", fixtureCase.CaseID, support.ExperienceID, primary.ExperienceID)
			}
			if support.FlakeDiscount <= 0 {
				t.Fatalf("flake fixture %s support %s is not flake-discounted", fixtureCase.CaseID, support.ExperienceID)
			}
			occurrences = append(occurrences, support)
			pathFingerprints[demoPathFingerprint(support.Paths)] = true
		}
		if len(occurrences) < 3 {
			t.Fatalf("flake fixture %s has %d repeated failures, want at least 3", fixtureCase.CaseID, len(occurrences))
		}
		if len(pathFingerprints) < 3 {
			t.Fatalf("flake fixture %s does not span unrelated changed paths: %#v", fixtureCase.CaseID, occurrences)
		}
	}
}

func demoPathFingerprint(paths []string) string {
	copied := append([]string(nil), paths...)
	sort.Strings(copied)
	return strings.Join(copied, "\x00")
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
