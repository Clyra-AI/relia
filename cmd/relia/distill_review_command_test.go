package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	configdoc "github.com/Clyra-AI/relia/internal/config"
)

func TestDistillDraftsDeterministicCandidateRulesReviewAndMemoryPage(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	writeFileForTest(t, filepath.Join(tempDir, "packages", "search", "query.py"), "def query():\n    return []\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0101","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":101,"commit":"abc101","paths":["packages/billing/invoice.py","tests/billing/test_invoice.py"],"actor_kind":"agent","attribution_method":"coauthor_trailer","attribution_confidence":0.91,"outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_clock","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/101"]}`,
		`{"experience_id":"exp_0102","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-15T10:00:00Z","pr":102,"commit":"abc102","paths":["packages/billing/invoice.py","tests/billing/test_invoice.py"],"actor_kind":"agent","attribution_method":"coauthor_trailer","attribution_confidence":0.91,"outcome_kind":"revert","terminal_state":"reverted","signature_id":"sig_billing_clock","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"log_parsed_high","flake_discount":0.25,"provenance_urls":["https://github.com/acme/billing-service/pull/102"]}`,
		`{"experience_id":"exp_0110","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-20T10:00:00Z","pr":110,"commit":"abc110","paths":["packages/search/query.py","tests/search/test_query.py"],"actor_kind":"agent","attribution_method":"pr_label","attribution_confidence":0.9,"outcome_kind":"fix_held","terminal_state":"held","signature_id":"sig_search_escape","signature_class":"test_failure","check_name":"pytest-search","signature_key":"tests/search/test_query.py::test_escape","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/110"]}`,
		`{"experience_id":"exp_0111","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-22T10:00:00Z","pr":111,"commit":"abc111","paths":["packages/search/query.py","tests/search/test_query.py"],"actor_kind":"agent","attribution_method":"pr_label","attribution_confidence":0.9,"outcome_kind":"merged_clean","terminal_state":"passed","signature_id":"sig_search_escape","signature_class":"test_failure","check_name":"pytest-search","signature_key":"tests/search/test_query.py::test_escape","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/111"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "distill" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	if result.RedactionStatus != "applied" {
		t.Fatalf("redaction_status = %q", result.RedactionStatus)
	}
	if got := int(result.Data["rules_written"].(float64)); got != 2 {
		t.Fatalf("rules_written = %d", got)
	}
	if got := int(result.Data["active_rules"].(float64)); got != 0 {
		t.Fatalf("active_rules = %d, want review gate to keep drafts inactive", got)
	}
	rules := loadRuleDocsByKindForTest(t, tempDir)
	avoid := rules["avoid"]
	playbook := rules["playbook"]
	if avoid.Scalars["status"].Value != "candidate" || avoid.Scalars["review.label"].Value != "suggested" {
		t.Fatalf("avoid lifecycle = status %q review %q", avoid.Scalars["status"].Value, avoid.Scalars["review.label"].Value)
	}
	if playbook.Scalars["status"].Value != "candidate" || playbook.Scalars["review.label"].Value != "suggested" {
		t.Fatalf("playbook lifecycle = status %q review %q", playbook.Scalars["status"].Value, playbook.Scalars["review.label"].Value)
	}
	if avoid.Scalars["review.statement_origin"].Value != "cluster_summary" || playbook.Scalars["review.statement_origin"].Value != "cluster_summary" {
		t.Fatalf("statement origins = avoid %q playbook %q", avoid.Scalars["review.statement_origin"].Value, playbook.Scalars["review.statement_origin"].Value)
	}
	if avoid.Scalars["metadata.confidence_inputs.drafting_model_weight"].Value != "0" {
		t.Fatalf("drafting model affected confidence: %#v", avoid.Scalars["metadata.confidence_inputs.drafting_model_weight"])
	}
	if avoid.Scalars["metadata.confidence_inputs.evidence_count"].Value != "2" ||
		avoid.Scalars["metadata.confidence_inputs.contradictions"].Value != "0" ||
		avoid.Scalars["metadata.confidence_inputs.flake_discount"].Value == "0" ||
		avoid.Scalars["metadata.decay.half_life_days"].Value != "90" {
		t.Fatalf("avoid confidence metadata = %#v", avoid.Scalars)
	}
	if avoid.Scalars["metadata.memory_source"].Value != "verified_outcome_events" ||
		avoid.Scalars["metadata.source_record_type"].Value != "relia.experience_record" {
		t.Fatalf("avoid memory source metadata = %#v", avoid.Scalars)
	}
	if got := yamlScalarValuesForTest(avoid.Lists["metadata.excluded_memory_sources"]); !stringSlicesEqual(got, []string{"agent_self_report", "agent_reflection"}) {
		t.Fatalf("excluded memory sources = %#v", got)
	}
	firstAvoidContent := readRuleByIDForTest(t, tempDir, avoid.Scalars["id"].Value)
	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("second distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	secondAvoidContent := readRuleByIDForTest(t, tempDir, avoid.Scalars["id"].Value)
	if firstAvoidContent != secondAvoidContent {
		t.Fatalf("distill was not deterministic:\nfirst=%s\nsecond=%s", firstAvoidContent, secondAvoidContent)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "review", "--rule", avoid.Scalars["id"].Value, "--label", "accepted"}, false)
	if code != ExitSuccess {
		t.Fatalf("review exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	reviewed := parseRuleDocForTest(t, readRuleByIDForTest(t, tempDir, avoid.Scalars["id"].Value))
	if reviewed.Scalars["status"].Value != "active" || reviewed.Scalars["review.label"].Value != "accepted" {
		t.Fatalf("reviewed lifecycle = status %q review %q", reviewed.Scalars["status"].Value, reviewed.Scalars["review.label"].Value)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "memory", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("memory exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	page, err := os.ReadFile(filepath.Join(tempDir, "memory", "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Relia Memory",
		"active",
		"candidate",
		"confidence",
		"[PR #101](https://github.com/acme/billing-service/pull/101)",
		"[PR #110](https://github.com/acme/billing-service/pull/110)",
	} {
		if !strings.Contains(string(page), want) {
			t.Fatalf("MEMORY.md missing %q:\n%s", want, page)
		}
	}
	for _, want := range []string{
		"## Strong Memory",
		"## Weak Memory",
		"Active accepted rules",
		"Candidate, stale, contradicted, and retired",
	} {
		if !strings.Contains(string(page), want) {
			t.Fatalf("MEMORY.md missing weak/strong separation marker %q:\n%s", want, page)
		}
	}
}

func TestDistillInputDraftsAvoidRuleFromPlantedRecurrenceCluster(t *testing.T) {
	sourceRoot := findRepoRootForTest(t)
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	fixtureRel := filepath.Join("examples", "demo", "seeded-repo", "outcomes.jsonl")
	fixtureContent, err := os.ReadFile(filepath.Join(sourceRoot, fixtureRel))
	if err != nil {
		t.Fatal(err)
	}
	writeFileForTest(t, filepath.Join(tempDir, fixtureRel), string(fixtureContent))
	for _, record := range decodeJSONLines(t, string(fixtureContent)) {
		for _, path := range stringListField(record, "paths", "context.paths") {
			clean, ok := configdoc.CleanRepoPath(path)
			if !ok {
				t.Fatalf("fixture path is not repo-relative: %q", path)
			}
			writeFileForTest(t, filepath.Join(tempDir, filepath.FromSlash(filepath.ToSlash(clean))), "fixture path\n")
		}
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", filepath.ToSlash(fixtureRel), "--format", "json"}, false)

	if code != ExitSuccess {
		t.Fatalf("distill input exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got, _ := result.Data["input_path"].(string); got != filepath.ToSlash(fixtureRel) {
		t.Fatalf("input_path = %q, want %q", got, filepath.ToSlash(fixtureRel))
	}
	if got := int(result.Data["rules_written"].(float64)); got < 1 {
		t.Fatalf("rules_written = %d, want at least one rule from planted fixture", got)
	}
	wantExperienceIDs := []string{"exp_0142", "exp_0187", "exp_0203"}
	avoid := findRuleDocByEvidenceForTest(t, tempDir, "avoid", wantExperienceIDs)
	if avoid.Scalars["status"].Value != "candidate" ||
		avoid.Scalars["review.label"].Value != "suggested" ||
		avoid.Scalars["evidence.count"].Value != "3" ||
		avoid.Scalars["evidence.contradictions"].Value != "0" {
		t.Fatalf("avoid rule lifecycle/evidence = %#v", avoid.Scalars)
	}
	if got := yamlScalarValuesForTest(avoid.Lists["evidence.experiences"]); !stringSlicesEqual(got, wantExperienceIDs) {
		t.Fatalf("avoid evidence experiences = %#v, want %#v", got, wantExperienceIDs)
	}
	wantCitations := map[int]string{
		142: "https://github.com/Clyra-AI/relia-demo-seed/pull/142",
		187: "https://github.com/Clyra-AI/relia-demo-seed/pull/187",
		203: "https://github.com/Clyra-AI/relia-demo-seed/pull/203",
	}
	for _, entry := range avoid.ListMaps["provenance"] {
		pr, err := strconv.Atoi(entry["pr"].Value)
		if err != nil {
			t.Fatalf("provenance pr = %#v", entry["pr"])
		}
		if wantCitations[pr] != entry["url"].Value {
			t.Fatalf("provenance entry = %#v, want PR citation %q", entry, wantCitations[pr])
		}
		delete(wantCitations, pr)
	}
	if len(wantCitations) != 0 {
		t.Fatalf("missing planted recurrence citations: %#v", wantCitations)
	}
}

func TestDistillRejectsCanonicalSelfReportBeforeMemoryWrite(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "metadata_source_kind",
			mutate: func(record map[string]any) {
				metadata := record["metadata"].(map[string]any)
				metadata["source_kind"] = "agent_reflection"
			},
		},
		{
			name: "metadata_event_type",
			mutate: func(record map[string]any) {
				metadata := record["metadata"].(map[string]any)
				metadata["event_type"] = "agent_reflection"
			},
		},
		{
			name: "metadata_source_object_type",
			mutate: func(record map[string]any) {
				metadata := record["metadata"].(map[string]any)
				metadata["source"] = map[string]any{"object_type": "agent_self_report"}
			},
		},
		{
			name: "camel_case_metadata_source_kind",
			mutate: func(record map[string]any) {
				metadata := record["metadata"].(map[string]any)
				metadata["source_kind"] = "agentSelfReport"
			},
		},
		{
			name: "top_level_event_type",
			mutate: func(record map[string]any) {
				record["event_type"] = "agent_reflection"
			},
		},
		{
			name: "top_level_source_kind",
			mutate: func(record map[string]any) {
				record["source_kind"] = "agent_self_report"
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := setupContractRepo(t)
			t.Chdir(tempDir)
			writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
			inputRel := filepath.ToSlash(filepath.Join("fixtures", "self-report-experience-records.jsonl"))
			inputPath := filepath.Join(tempDir, filepath.FromSlash(inputRel))
			record := canonicalExperienceRecordMapForTest("exp_self_report_001", 521)
			tc.mutate(record)
			content, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			writeFileForTest(t, inputPath, string(content)+"\n")

			stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)

			if code != ExitValidation {
				t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if result.Errors[0].Type != "artifact_contract_validation_failed" ||
				!strings.Contains(result.Errors[0].Message, "self-reports") {
				t.Fatalf("errors = %#v", result.Errors)
			}
			matches, err := filepath.Glob(filepath.Join(tempDir, "memory", "rules", "*.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 0 {
				t.Fatalf("distill wrote memory rules from an agent self-report: %#v", matches)
			}
		})
	}
}

func TestDistillSeparatesCanonicalSignatureClustersByCheckName(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-canonical-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0501","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":501,"commit":"abc501","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_a","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/501"]}`,
		`{"experience_id":"exp_0502","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-08T10:00:00Z","pr":502,"commit":"abc502","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_generated_b","signature_class":"test_failure","check_name":"pytest-billing-rerun","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/502"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got := int(result.Data["rules_written"].(float64)); got != 2 {
		t.Fatalf("rules_written = %d, want separate class/check/key clusters", got)
	}
	wantEvidence := map[string]bool{"exp_0501": false, "exp_0502": false}
	for _, rule := range loadRuleDocsForTest(t, tempDir) {
		if rule.Scalars["kind"].Value != "avoid" {
			continue
		}
		if rule.Scalars["evidence.count"].Value != "1" {
			t.Fatalf("evidence.count = %q, want one record per class/check/key cluster", rule.Scalars["evidence.count"].Value)
		}
		experiences := yamlScalarValuesForTest(rule.Lists["evidence.experiences"])
		if len(experiences) != 1 {
			t.Fatalf("evidence experiences = %#v, want one record per separated check", experiences)
		}
		if _, ok := wantEvidence[experiences[0]]; !ok {
			t.Fatalf("unexpected evidence experience %q", experiences[0])
		}
		wantEvidence[experiences[0]] = true
		confidence, err := strconv.ParseFloat(rule.Scalars["confidence"].Value, 64)
		if err != nil {
			t.Fatal(err)
		}
		if confidence > 0.6 {
			t.Fatalf("confidence = %.4f, want capped at 0.6 until three confirmed experiences", confidence)
		}
		if rule.Scalars["metadata.embedding_mode"].Value != "signature" ||
			rule.Scalars["metadata.cluster.provenance"].Value != "signature_only" {
			t.Fatalf("signature fallback provenance metadata = %#v", rule.Scalars)
		}
	}
	for experienceID, seen := range wantEvidence {
		if !seen {
			t.Fatalf("missing separated avoid rule for %s", experienceID)
		}
	}
}

func TestDistillInputPreservesCanonicalExperienceSignatureMetadata(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	writeFileForTest(t, filepath.Join(tempDir, "packages", "payments", "clock.py"), "def now():\n    return 1\n")
	inputRel := filepath.ToSlash(filepath.Join("fixtures", "distill-canonical-experience-records.jsonl"))
	inputPath := filepath.Join(tempDir, filepath.FromSlash(inputRel))
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"object_type":"relia.experience_record","schema_version":"1.0","experience_id":"exp_0521","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","attribution":{"actor_kind":"agent","method":"manual","confidence":1},"context":{"paths":["packages/billing/invoice.py"],"diff_fingerprint":"sha256:canonical-a"},"action":{"pr":521,"commit":"abc521"},"outcome":{"kind":"ci_failure","terminal_state":"failed","signature":{"signature_id":"sig_canonical_a","extraction_confidence":"structured"}},"provenance":{"urls":["https://github.com/acme/billing-service/pull/521"]},"flake_discount":0,"org_eligible":false,"share_scope":"private","redaction_status":"applied","metadata":{"signature":{"class":"test_failure","check_name":"pytest-billing","key":"tests/billing/test_invoice.py::test_clock","message_fingerprint":"sha256:canonical-shared-input","extraction_method":"structured"},"source_kind":"ingest"}}`,
		`{"object_type":"relia.experience_record","schema_version":"1.0","experience_id":"exp_0522","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-08T10:00:00Z","attribution":{"actor_kind":"agent","method":"manual","confidence":1},"context":{"paths":["packages/payments/clock.py"],"diff_fingerprint":"sha256:canonical-b"},"action":{"pr":522,"commit":"abc522"},"outcome":{"kind":"ci_failure","terminal_state":"failed","signature":{"signature_id":"sig_canonical_b","extraction_confidence":"structured"}},"provenance":{"urls":["https://github.com/acme/billing-service/pull/522"]},"flake_discount":0,"org_eligible":false,"share_scope":"private","redaction_status":"applied","metadata":{"signature":{"class":"test_failure","check_name":"go-test-payments","key":"tests/payments/test_clock.py::test_clock","message_fingerprint":"sha256:canonical-shared-input","extraction_method":"structured"},"source_kind":"ingest"}}`,
	}, "\n")+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill input exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got := int(result.Data["rules_written"].(float64)); got != 1 {
		t.Fatalf("rules_written = %d, want one canonical message-fingerprint cluster", got)
	}
	avoid := loadRuleDocsByKindForTest(t, tempDir)["avoid"]
	if got := yamlScalarValuesForTest(avoid.Lists["evidence.experiences"]); !stringSlicesEqual(got, []string{"exp_0521", "exp_0522"}) {
		t.Fatalf("avoid evidence experiences = %#v, want canonical input message-fingerprint cluster", got)
	}
	if got := avoid.Scalars["metadata.cluster.key"].Value; got != "message|sha256:canonical-shared-input" {
		t.Fatalf("metadata.cluster.key = %q, want canonical message fingerprint key", got)
	}
}

func TestDistillInputRejectsOrgEligibleCanonicalExperience(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	inputRel := filepath.ToSlash(filepath.Join("fixtures", "distill-org-eligible-canonical-experience.jsonl"))
	inputPath := filepath.Join(tempDir, filepath.FromSlash(inputRel))
	writeFileForTest(t, inputPath, `{"object_type":"relia.experience_record","schema_version":"1.0","experience_id":"exp_0523","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","attribution":{"actor_kind":"agent","method":"manual","confidence":1},"context":{"paths":["packages/billing/invoice.py"],"diff_fingerprint":"sha256:canonical-org"},"action":{"pr":523,"commit":"abc523"},"outcome":{"kind":"ci_failure","terminal_state":"failed","signature":{"signature_id":"sig_canonical_org","extraction_confidence":"structured"}},"provenance":{"urls":["https://github.com/acme/billing-service/pull/523"]},"flake_discount":0,"org_eligible":true,"share_scope":"private","redaction_status":"applied","metadata":{"signature":{"class":"test_failure","check_name":"pytest-billing","key":"tests/billing/test_invoice.py::test_clock","message_fingerprint":"sha256:canonical-org","extraction_method":"structured"},"source_kind":"ingest"}}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)
	if code != ExitValidation {
		t.Fatalf("distill org-eligible input exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "org_eligible must be false") {
		t.Fatalf("distill org-eligible errors = %#v", result.Errors)
	}
}

func TestDistillInputRejectsIncompleteCanonicalExperience(t *testing.T) {
	tests := []struct {
		name        string
		removePath  []string
		wantMessage string
	}{
		{name: "commit", removePath: []string{"action", "commit"}, wantMessage: "action.commit must be provided"},
		{name: "method", removePath: []string{"attribution", "method"}, wantMessage: "attribution.method must be provided"},
		{name: "confidence", removePath: []string{"attribution", "confidence"}, wantMessage: "attribution.confidence must be provided"},
		{name: "diff", removePath: []string{"context", "diff_fingerprint"}, wantMessage: "context.diff_fingerprint must be provided"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := setupContractRepo(t)
			t.Chdir(tempDir)
			inputRel := filepath.ToSlash(filepath.Join("fixtures", "distill-incomplete-canonical-experience.jsonl"))
			inputPath := filepath.Join(tempDir, filepath.FromSlash(inputRel))
			record := canonicalExperienceRecordMapForTest("exp_0524", 524)
			deleteNestedMapFieldForTest(record, tc.removePath...)
			content, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			writeFileForTest(t, inputPath, string(content)+"\n")

			stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)
			if code != ExitValidation {
				t.Fatalf("distill incomplete input exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
			}
			result := decodeResult(t, stdout)
			if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, tc.wantMessage) {
				t.Fatalf("distill incomplete errors = %#v, want %q", result.Errors, tc.wantMessage)
			}
		})
	}
}

func canonicalExperienceRecordMapForTest(experienceID string, pr int) map[string]any {
	return map[string]any{
		"object_type":    "relia.experience_record",
		"schema_version": commandSchemaVersion,
		"experience_id":  experienceID,
		"repo": map[string]any{
			"provider": "github",
			"owner":    "acme",
			"name":     "billing-service",
		},
		"recorded_at": "2026-04-01T10:00:00Z",
		"attribution": map[string]any{
			"actor_kind": "agent",
			"method":     "manual",
			"confidence": 1,
		},
		"context": map[string]any{
			"paths":            []any{"packages/billing/invoice.py"},
			"diff_fingerprint": "sha256:canonical-complete",
		},
		"action": map[string]any{
			"pr":     pr,
			"commit": "abc524",
		},
		"outcome": map[string]any{
			"kind":           "ci_failure",
			"terminal_state": "failed",
			"signature": map[string]any{
				"signature_id":          "sig_canonical_complete",
				"extraction_confidence": "structured",
			},
		},
		"provenance": map[string]any{
			"urls": []any{fmt.Sprintf("https://github.com/acme/billing-service/pull/%d", pr)},
		},
		"flake_discount":   0,
		"org_eligible":     false,
		"share_scope":      "private",
		"redaction_status": "applied",
		"metadata": map[string]any{
			"signature": map[string]any{
				"class":               "test_failure",
				"check_name":          "pytest-billing",
				"key":                 "tests/billing/test_invoice.py::test_clock",
				"message_fingerprint": "sha256:canonical-complete",
				"extraction_method":   "structured",
			},
			"source_kind": "ingest",
		},
	}
}

func deleteNestedMapFieldForTest(root map[string]any, path ...string) {
	current := root
	for _, key := range path[:len(path)-1] {
		next, _ := current[key].(map[string]any)
		current = next
	}
	delete(current, path[len(path)-1])
}

func TestDistillStableIDCheckKeyClustersPositiveEvidence(t *testing.T) {
	failure := distillClusterKeyForTest("ci_failure", "sig_time_freeze", "test_failure", "pytest-billing", "tests/billing/test_invoice_time.py::test_rollover_uses_frozen_clock")
	revert := distillClusterKeyForTest("revert", "sig_time_freeze", "revert", "pytest-billing", "tests/billing/test_invoice_time.py::test_rollover_uses_frozen_clock")
	held := distillClusterKeyForTest("fix_held", "sig_time_freeze", "held_fix", "pytest-billing", "tests/billing/test_invoice_time.py::test_rollover_uses_frozen_clock")
	if failure == "" || revert == "" {
		t.Fatalf("failure key = %q revert key = %q, want non-empty keys", failure, revert)
	}
	if failure != revert {
		t.Fatalf("failure key = %q revert key = %q, want stable signature ID/check/key to co-cluster related outcomes", failure, revert)
	}
	if held != failure {
		t.Fatalf("held fix key = %q failure key = %q, want stable ID/check/key to keep positive evidence attached to the same distill cluster", held, failure)
	}
}

func TestDistillSeparatesReusedStableSignatureIDsByCheckName(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-reused-stable-id-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0551","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":551,"commit":"abc551","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_reused_monorepo","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/551"]}`,
		`{"experience_id":"exp_0552","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-08T10:00:00Z","pr":552,"commit":"abc552","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_reused_monorepo","signature_class":"test_failure","check_name":"go-test-billing","signature_key":"tests/billing/test_invoice.py::test_clock","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/552"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got := int(result.Data["rules_written"].(float64)); got != 2 {
		t.Fatalf("rules_written = %d, want reused stable signature ID separated by check_name", got)
	}
}

func TestDistillClustersMatchingMessageFingerprintsAcrossChecks(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-message-fingerprint-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0601","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":601,"commit":"abc601","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_clock","message_fingerprint":"sha256:shared-clock-failure","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/601"]}`,
		`{"experience_id":"exp_0602","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-08T10:00:00Z","pr":602,"commit":"abc602","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_class":"test_failure","check_name":"go-test-billing","signature_key":"tests/billing/test_invoice.py::test_clock","message_fingerprint":"sha256:shared-clock-failure","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/602"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got := int(result.Data["rules_written"].(float64)); got != 1 {
		t.Fatalf("rules_written = %d, want one message-fingerprint cluster", got)
	}
	avoid := loadRuleDocsByKindForTest(t, tempDir)["avoid"]
	if got := yamlScalarValuesForTest(avoid.Lists["evidence.experiences"]); !stringSlicesEqual(got, []string{"exp_0601", "exp_0602"}) {
		t.Fatalf("avoid evidence experiences = %#v, want message-fingerprint cluster", got)
	}
	if got := avoid.Scalars["metadata.cluster.key"].Value; got != "message|sha256:shared-clock-failure" {
		t.Fatalf("metadata.cluster.key = %q, want shared message fingerprint key", got)
	}
}

func TestDistillRejectsBlankExplicitInput(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", " ", "--format", "json"}, false)

	if code != ExitUsage {
		t.Fatalf("distill blank input exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "distill --input must be a non-empty path") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestDistillReviewRequiredFalseStillDraftsCandidateRules(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "review_required: true", "review_required: false")
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-review-gate-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0701","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":701,"commit":"abc701","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_review_gate","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_review_gate","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/701"]}`,
		`{"experience_id":"exp_0702","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-08T10:00:00Z","pr":702,"commit":"abc702","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_review_gate","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_review_gate","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/702"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	avoid := loadRuleDocsByKindForTest(t, tempDir)["avoid"]
	if avoid.Scalars["status"].Value != "candidate" || avoid.Scalars["review.label"].Value == "accepted" {
		t.Fatalf("review_required=false auto-accepted draft: status %q review %q", avoid.Scalars["status"].Value, avoid.Scalars["review.label"].Value)
	}
	if avoid.Scalars["metadata.review_required"].Value != "false" {
		t.Fatalf("review_required metadata = %#v", avoid.Scalars["metadata.review_required"])
	}
}

func TestCheckDisclosesOpenAICompatibleProviderBoundary(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	enableProviderForTest(t, tempDir, "openai_compatible", "gpt-test", "https://openai-compatible.example.test/v1", "RELIA_OPENAI_COMPATIBLE_API_KEY", "5.00")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("check exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Warnings) == 0 {
		t.Fatal("expected provider disclosure warning")
	}
	var disclosure Finding
	for _, warning := range result.Warnings {
		if warning.Type == "provider_data_disclosure" {
			disclosure = warning
			break
		}
	}
	if disclosure.Type == "" || !strings.Contains(disclosure.Message, "redacted experience records") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestCheckRejectsProviderBaseURLUserInfo(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	enableProviderForTest(t, tempDir, "openai_compatible", "gpt-test", "https://token@openai-compatible.example.test/v1", "RELIA_OPENAI_COMPATIBLE_API_KEY", "5.00")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitUsage {
		t.Fatalf("check user-info URL exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "must not include user info") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if strings.Contains(stdout, "token@") || strings.Contains(stderr, "token@") {
		t.Fatalf("provider URL user info leaked in output: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestDistillProviderPlanReportsCostAndFailsClosedWithoutGrant(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	enableProviderForTest(t, tempDir, "anthropic", "claude-test", "https://api.anthropic.example.test", "RELIA_ANTHROPIC_API_KEY", "5.00")
	inputRel := filepath.ToSlash(filepath.Join("fixtures", "provider-distill.jsonl"))
	writeFileForTest(t, filepath.Join(tempDir, filepath.FromSlash(inputRel)), `{"experience_id":"exp_0901","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":901,"commit":"abc901","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_provider_plan","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_provider","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/901"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)

	if code != ExitDependency {
		t.Fatalf("distill provider exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "model_provider_endpoint") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	plan := result.Data["provider_plan"].(map[string]any)
	if plan["provider"] != "anthropic" || plan["adapter"] != "anthropic_messages_http" || plan["model"] != "claude-test" {
		t.Fatalf("provider_plan = %#v", plan)
	}
	cost := plan["cost"].(map[string]any)
	if cost["estimated_cost_usd"].(float64) <= 0 ||
		cost["input_tokens_estimated"].(float64) <= 0 ||
		cost["output_tokens_estimated"].(float64) <= 0 {
		t.Fatalf("cost estimate = %#v", cost)
	}
	if plan["provider_call_attempted"] != false || plan["approval_required"] != "model_provider_endpoint" {
		t.Fatalf("provider execution boundary = %#v", plan)
	}
	if matches, err := filepath.Glob(filepath.Join(tempDir, "memory", "rules", "*.yaml")); err != nil {
		t.Fatal(err)
	} else if len(matches) != 0 {
		t.Fatalf("provider-gated distill wrote memory rules without grant: %#v", matches)
	}
}

func TestDistillProviderPlanRespectsConfiguredCostCap(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	enableProviderForTest(t, tempDir, "anthropic", "claude-test", "https://api.anthropic.example.test", "RELIA_ANTHROPIC_API_KEY", "0.000001")
	inputRel := filepath.ToSlash(filepath.Join("fixtures", "provider-cost-cap.jsonl"))
	writeFileForTest(t, filepath.Join(tempDir, filepath.FromSlash(inputRel)), `{"experience_id":"exp_0911","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":911,"commit":"abc911","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_provider_cap","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_provider_cap","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/911"]}`+"\n")

	stdout, stderr, code := runForTest(t, []string{"--json", "distill", "--input", inputRel, "--format", "json"}, false)

	if code != ExitDependency {
		t.Fatalf("distill provider cap exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "max_cost_usd_per_run") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	cost := result.Data["provider_plan"].(map[string]any)["cost"].(map[string]any)
	if cost["cap_status"] != "exceeded" {
		t.Fatalf("cost estimate = %#v", cost)
	}
}

func TestReviewApproveEditRejectTransitions(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	rulePath := filepath.Join(tempDir, "memory", "rules", "billing-time.yaml")
	writeFileForTest(t, rulePath, `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-time
kind: avoid
status: candidate
statement: Avoid direct billing clock calls.
scope:
  paths:
    - packages/billing/invoice.py
confidence: 0.6
evidence:
  count: 2
  contradictions: 0
  experiences:
    - exp_0601
    - exp_0602
provenance:
  - pr: 601
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/601
review:
  label: suggested
  statement_origin: cluster_summary
metadata:
  confidence_label: medium
  lifecycle_reason: human review required before activation
  confidence_inputs:
    evidence_count: 2
    recency_weight: 1
    contradictions: 0
    flake_discount: 0
    extraction_confidence: 1
    drafting_model_weight: 0
  decay:
    half_life_days: 90
    latest_evidence_at: 2026-04-08T10:00:00Z
    oldest_evidence_at: 2026-04-01T10:00:00Z
    anchor_recorded_at: 2026-04-08T10:00:00Z
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "review", "edit", "--rule", "billing-time", "--statement", "Use the billing clock fixture instead of direct UTC calls."}, false)
	if code != ExitSuccess {
		t.Fatalf("review edit exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	edited := parseRuleDocForTest(t, readRuleByIDForTest(t, tempDir, "billing-time"))
	if edited.Scalars["status"].Value != "candidate" ||
		edited.Scalars["review.label"].Value != "suggested" ||
		edited.Scalars["review.statement_origin"].Value != "human_authored" ||
		edited.Scalars["statement"].Value != "Use the billing clock fixture instead of direct UTC calls." {
		t.Fatalf("edited rule = %#v", edited.Scalars)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "review", "approve", "--rule", "billing-time"}, false)
	if code != ExitSuccess {
		t.Fatalf("review approve exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	approved := parseRuleDocForTest(t, readRuleByIDForTest(t, tempDir, "billing-time"))
	if approved.Scalars["status"].Value != "active" || approved.Scalars["review.label"].Value != "accepted" {
		t.Fatalf("approved rule = %#v", approved.Scalars)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "review", "reject", "--rule", "billing-time", "--reason", "superseded by a narrower billing rule"}, false)
	if code != ExitSuccess {
		t.Fatalf("review reject exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	rejected := parseRuleDocForTest(t, readRuleByIDForTest(t, tempDir, "billing-time"))
	if rejected.Scalars["status"].Value != "retired" ||
		rejected.Scalars["review.label"].Value != "needs_user_input" ||
		!strings.Contains(rejected.Scalars["metadata.lifecycle_reason"].Value, "superseded") {
		t.Fatalf("rejected rule = %#v", rejected.Scalars)
	}
}

func TestDistillMarksContradictedAndStaleRules(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-lifecycle-outcomes.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0201","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-03-01T10:00:00Z","pr":201,"commit":"abc201","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_rounding","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/201"]}`,
		`{"experience_id":"exp_0202","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-03-08T10:00:00Z","pr":202,"commit":"abc202","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"merged_clean","terminal_state":"passed","signature_id":"sig_billing_rounding","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/202"]}`,
		`{"experience_id":"exp_0301","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-04-01T10:00:00Z","pr":301,"commit":"abc301","paths":["packages/removed/legacy.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_removed_legacy","signature_class":"test_failure","check_name":"pytest-legacy","signature_key":"tests/legacy/test_removed.py::test_legacy","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/301"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	rules := loadRuleDocsByStatusForTest(t, tempDir)
	contradicted := rules["contradicted"]
	stale := rules["stale"]
	if contradicted.Scalars["review.label"].Value != "needs_user_input" ||
		contradicted.Scalars["evidence.contradictions"].Value != "1" {
		t.Fatalf("contradicted rule = %#v", contradicted.Scalars)
	}
	if stale.Scalars["metadata.lifecycle_reason"].Value != "all scoped paths are missing from the working tree" {
		t.Fatalf("stale rule metadata = %#v", stale.Scalars)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "review", "--rule", stale.Scalars["id"].Value, "--label", "accepted"}, false)
	if code != ExitValidation {
		t.Fatalf("review stale exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" || !strings.Contains(result.Errors[0].Message, "stale") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestReviewFailsClosedWithoutMutatingInvalidRule(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	rulePath := filepath.Join(tempDir, "memory", "rules", "invalid-candidate.yaml")
	writeFileForTest(t, rulePath, `object_type: relia.memory_rule
schema_version: "1.0"
id: invalid-candidate
kind: avoid
status: candidate
statement: Avoid direct billing clock calls.
scope:
  paths:
    - packages/billing/invoice.py
confidence: 0.72
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0501
review:
  label: suggested
  statement_origin: cluster_summary
metadata:
  confidence_label: medium
`)
	before, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "review", "--rule", "invalid-candidate", "--label", "accepted"}, false)

	if code != ExitValidation {
		t.Fatalf("review exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" || !strings.Contains(result.Errors[0].Message, "provenance") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	after, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("review mutated invalid rule despite failed validation:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestReviewRejectsEditOnlyFlagsWithoutEditAction(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)

	stdout, stderr, code := runForTest(t, []string{"--json", "review", "--rule", "billing-time", "--statement", "Use a safer billing clock."}, false)

	if code != ExitUsage {
		t.Fatalf("review edit-only flag exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" || !strings.Contains(result.Errors[0].Message, "require review edit") {
		t.Fatalf("errors = %#v", result.Errors)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "review", "--rule", "billing-time", "--reason", "not enough evidence"}, false)
	if code != ExitUsage {
		t.Fatalf("review reason-only flag exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result = decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" || !strings.Contains(result.Errors[0].Message, "requires review reject") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestReviewEditRejectsMissingScopePathWithoutMutatingRule(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	rulePath := filepath.Join(tempDir, "memory", "rules", "billing-time.yaml")
	writeFileForTest(t, rulePath, `object_type: relia.memory_rule
schema_version: "1.0"
id: billing-time
kind: avoid
status: candidate
statement: Avoid direct billing clock calls.
scope:
  paths:
    - packages/billing/invoice.py
confidence: 0.6
evidence:
  count: 1
  contradictions: 0
  experiences:
    - exp_0601
provenance:
  - pr: 601
    outcome: ci_failure
    url: https://github.com/acme/billing-service/pull/601
review:
  label: suggested
  statement_origin: cluster_summary
metadata:
  confidence_label: medium
  confidence_inputs:
    evidence_count: 1
    recency_weight: 1
    contradictions: 0
    flake_discount: 0
    extraction_confidence: 1
    drafting_model_weight: 0
  decay:
    half_life_days: 90
    latest_evidence_at: 2026-04-08T10:00:00Z
    oldest_evidence_at: 2026-04-01T10:00:00Z
    anchor_recorded_at: 2026-04-08T10:00:00Z
`)
	before, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "review", "edit", "--rule", "billing-time", "--scope-path", "packages/billing/missing.py"}, false)

	if code != ExitValidation {
		t.Fatalf("review edit missing scope exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" || !strings.Contains(result.Errors[0].Message, "scope path does not exist") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	after, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("review edit mutated rule despite invalid scope:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestDistillMarksAnyLaterContradictionAsContradicted(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def total():\n    return 1\n")
	inputPath := filepath.Join(tempDir, "fixtures", "distill-later-contradiction.jsonl")
	writeFileForTest(t, inputPath, strings.Join([]string{
		`{"experience_id":"exp_0401","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-03-01T10:00:00Z","pr":401,"commit":"abc401","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_tax_rounding","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_tax_rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/401"]}`,
		`{"experience_id":"exp_0402","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-03-08T10:00:00Z","pr":402,"commit":"abc402","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"ci_failure","terminal_state":"failed","signature_id":"sig_billing_tax_rounding","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_tax_rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/402"]}`,
		`{"experience_id":"exp_0403","repo":{"provider":"github","owner":"acme","name":"billing-service"},"recorded_at":"2026-03-15T10:00:00Z","pr":403,"commit":"abc403","paths":["packages/billing/invoice.py"],"actor_kind":"agent","attribution_method":"manual","outcome_kind":"merged_clean","terminal_state":"passed","signature_id":"sig_billing_tax_rounding","signature_class":"test_failure","check_name":"pytest-billing","signature_key":"tests/billing/test_invoice.py::test_tax_rounding","extraction_confidence":"structured","provenance_urls":["https://github.com/acme/billing-service/pull/403"]}`,
	}, "\n")+"\n")
	stdout, stderr, code := runForTest(t, []string{"--json", "ingest", "--input", inputPath}, false)
	if code != ExitSuccess {
		t.Fatalf("ingest exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}

	stdout, stderr, code = runForTest(t, []string{"--json", "distill", "--format", "json"}, false)
	if code != ExitSuccess {
		t.Fatalf("distill exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	rules := loadRuleDocsByStatusForTest(t, tempDir)
	contradicted := rules["contradicted"]
	if contradicted.Scalars["evidence.contradictions"].Value != "1" ||
		contradicted.Scalars["review.label"].Value != "needs_user_input" {
		t.Fatalf("contradicted rule = %#v", contradicted.Scalars)
	}
}
