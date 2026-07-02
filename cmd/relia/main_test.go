package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	configdoc "github.com/Clyra-AI/relia/internal/config"
)

func TestJSONFlagEmitsStableEnvelope(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.ObjectType != "relia.command_result" {
		t.Fatalf("object_type = %q", result.ObjectType)
	}
	if result.SchemaVersion != "1.0" {
		t.Fatalf("schema_version = %q", result.SchemaVersion)
	}
	if result.Command != "check" {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
	if result.ExitCode != ExitSuccess {
		t.Fatalf("exit_code = %d", result.ExitCode)
	}
	if len(result.EvidenceRefs) == 0 {
		t.Fatal("expected evidence_refs to be preserved")
	}
	if result.RedactionStatus == "" {
		t.Fatal("expected redaction_status")
	}
	if result.Metadata["schema_ref"] != "schemas/command-result.schema.json" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestPipedOutputDefaultsToJSON(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "check" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
}

func TestInteractiveOutputIsHumanReadable(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"check"}, true)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	if json.Valid([]byte(stdout)) {
		t.Fatalf("interactive output should be human-readable, got JSON: %s", stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("pass check")) {
		t.Fatalf("interactive output = %q, want pass check", stdout)
	}
}

func TestQuietAndCompactPreserveMachineReadableEnvelope(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--quiet", "--compact", "check"}, true)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	if bytes.Contains([]byte(stdout), []byte("\n  ")) {
		t.Fatalf("compact output should not be indented: %q", stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
	if result.ExitCode != ExitSuccess {
		t.Fatalf("exit_code = %d", result.ExitCode)
	}
	if len(result.EvidenceRefs) == 0 {
		t.Fatal("quiet/compact output dropped evidence_refs")
	}
}

func TestUnknownCommandReturnsTypedUsageError(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "unknown-command"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "error" {
		t.Fatalf("status = %q", result.Status)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if result.Errors[0].Type != "invalid_usage" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if result.Errors[0].ExitCode != ExitUsage {
		t.Fatalf("error exit code = %d", result.Errors[0].ExitCode)
	}
}

func TestUnknownFlagReturnsTypedUsageError(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "--bogus"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestHelpAndVersionUseEnvelope(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "--help"},
		{"--json", "help"},
		{"--json", "--version"},
	} {
		stdout, stderr, code := runForTest(t, args, false)
		if code != ExitSuccess {
			t.Fatalf("%v exit code = %d, stderr = %q, stdout = %q", args, code, stderr, stdout)
		}
		result := decodeResult(t, stdout)
		if result.Status != "pass" {
			t.Fatalf("%v status = %q", args, result.Status)
		}
		if len(result.EvidenceRefs) == 0 {
			t.Fatalf("%v dropped evidence refs", args)
		}
	}
}

func TestReservedCommandsReturnTypedNotImplemented(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "compile"},
	} {
		stdout, stderr, code := runForTest(t, args, false)
		if code != ExitInternal {
			t.Fatalf("%v exit code = %d, stderr = %q, stdout = %q", args, code, stderr, stdout)
		}
		result := decodeResult(t, stdout)
		if result.Errors[0].Type != "not_implemented" {
			t.Fatalf("%v error type = %q", args, result.Errors[0].Type)
		}
	}
}

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
			clean, ok := cleanRepoPath(path)
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

func TestCheckRejectsZeroMatchAttributionConfigWithConcreteRef(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	configPath := filepath.Join(tempDir, "relia.yaml")
	replaceInFile(t, configPath, `  coauthor_trailers:
    - Claude
    - Claude Code`, `  coauthor_trailers: []`)
	replaceInFile(t, configPath, `  pr_labels:
    - agent-authored`, `  pr_labels: []`)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "zero agent matchers") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
	if !strings.Contains(result.Errors[0].Ref, "relia.yaml:") {
		t.Fatalf("error ref = %q, want concrete relia.yaml line", result.Errors[0].Ref)
	}
}

func TestCheckRejectsNaNRecurrenceGateThreshold(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	configPath := filepath.Join(tempDir, "relia.yaml")
	replaceInFile(t, configPath, `gate:
  enabled: false`, `gate:
  enabled: true
  max_error_recurrence_rate: NaN`)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if !strings.Contains(result.Errors[0].Message, "gate.max_error_recurrence_rate must be a number between 0 and 1") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
	if !strings.Contains(result.Errors[0].Ref, "relia.yaml:") {
		t.Fatalf("error ref = %q, want concrete relia.yaml line", result.Errors[0].Ref)
	}
}

func TestModelsRejectsUnsupportedSubcommand(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "models"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestInitCreatesBaselineConfig(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	stdout, stderr, code := runForTest(t, []string{"--json", "init"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "init" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	configPath := filepath.Join(tempDir, "relia.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected relia.yaml to be created: %v", err)
	}
	for _, token := range []string{"version: 1", "schema_version: \"1.0\"", "local_only: true", "fail_closed: true", "embeddings: signature", "advisory_only: true"} {
		if !bytes.Contains(content, []byte(token)) {
			t.Fatalf("relia.yaml missing %q:\n%s", token, content)
		}
	}
	for _, dir := range []string{".relia/experiences", ".relia/signatures", ".relia/coverage", ".relia/reports", ".relia/baselines", "memory/rules", "memory/compiled"} {
		if info, err := os.Stat(filepath.Join(tempDir, dir)); err != nil || !info.IsDir() {
			t.Fatalf("expected artifact skeleton dir %s: info=%#v err=%v", dir, info, err)
		}
	}
	ignoreContent, err := os.ReadFile(filepath.Join(tempDir, ".gitignore"))
	if err != nil {
		t.Fatalf("expected .gitignore to be created: %v", err)
	}
	if !bytes.Contains(ignoreContent, []byte(".relia/")) {
		t.Fatalf(".gitignore missing .relia/:\n%s", ignoreContent)
	}
}

func TestInitRejectsPositionalArguments(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "init", "extra"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestInitExistingConfigIsIdempotent(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	if err := os.WriteFile("relia.yaml", []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "init"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if created, ok := result.Data["created"].(bool); !ok || created {
		t.Fatalf("created = %#v, want false", result.Data["created"])
	}
}

func TestCheckFailsOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "local_configuration_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckReportsMissingOperatingPackFiles(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	if err := os.WriteFile("go.mod", []byte("module github.com/Clyra-AI/relia\n\ngo 1.26.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("relia.yaml", []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "operating_pack_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestHumanErrorWritesToStderr(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"unknown-command"}, true)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !bytes.Contains([]byte(stderr), []byte("error unknown-command")) {
		t.Fatalf("stderr = %q, want human error", stderr)
	}
}

func TestLowLevelHelpers(t *testing.T) {
	tempFile, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer tempFile.Close()

	if stdoutIsTerminal(tempFile) {
		t.Fatal("temporary file should not be detected as a terminal")
	}
	errResult := internalError("failed", errors.New("boom"))
	if errResult.ExitCode != ExitInternal || !bytes.Contains([]byte(errResult.Message), []byte("boom")) {
		t.Fatalf("internal error = %#v", errResult)
	}
}

func TestCommandResultExitCodeExamplesCoverStableCodes(t *testing.T) {
	root := findRepoRootForTest(t)
	content, err := os.ReadFile(filepath.Join(root, "examples", "command-results", "exit-code-examples.json"))
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		ObjectType    string          `json:"object_type"`
		SchemaVersion string          `json:"schema_version"`
		Examples      []CommandResult `json:"examples"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode examples: %v", err)
	}
	if payload.ObjectType != "relia.command_result_examples" {
		t.Fatalf("object_type = %q", payload.ObjectType)
	}
	codes := make([]int, 0, len(payload.Examples))
	for _, example := range payload.Examples {
		if example.ObjectType != "relia.command_result" {
			t.Fatalf("example object_type = %q", example.ObjectType)
		}
		if example.SchemaVersion != "1.0" {
			t.Fatalf("example schema_version = %q", example.SchemaVersion)
		}
		if example.Metadata["schema_version"] != "1.0" {
			t.Fatalf("example metadata = %#v", example.Metadata)
		}
		if example.ExitCode < ExitSuccess || example.ExitCode > ExitProvenanceIntegrity {
			t.Fatalf("unexpected exit code in example: %d", example.ExitCode)
		}
		codes = append(codes, example.ExitCode)
	}
	sort.Ints(codes)
	want := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	if len(codes) != len(want) {
		t.Fatalf("codes = %v, want %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("codes = %v, want %v", codes, want)
		}
	}
}

func TestPhase0SchemasDeclareMetadata(t *testing.T) {
	root := findRepoRootForTest(t)
	if commandErr := validateSchemaContracts(root); commandErr != nil {
		t.Fatalf("schema contracts failed: %#v", commandErr)
	}
}

func TestRecurrenceReportSchemaKeepsT8FieldsOptionalForV1(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(findRepoRootForTest(t), "schemas", "recurrence-report.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(content, &schema); err != nil {
		t.Fatal(err)
	}
	requiredValues, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema required field = %#v", schema["required"])
	}
	required := map[string]bool{}
	for _, value := range requiredValues {
		required[fmt.Sprint(value)] = true
	}
	for _, field := range []string{"metrics", "top_repeated_mistakes", "diagnostics", "operator_feedback", "badge"} {
		if required[field] {
			t.Fatalf("%s must stay optional while recurrence-report schema_version remains 1.0", field)
		}
	}
}

func TestCheckReportsPhase0ContractRefs(t *testing.T) {
	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if got, ok := result.Data["schema_contracts"].(float64); !ok || int(got) != len(requiredSchemaFiles) {
		t.Fatalf("schema_contracts = %#v, want %d", result.Data["schema_contracts"], len(requiredSchemaFiles))
	}
	if result.Data["privacy_default"] != "local_only" {
		t.Fatalf("privacy_default = %#v", result.Data["privacy_default"])
	}
	if len(result.Artifacts) <= len(requiredSchemaFiles) {
		t.Fatalf("expected schema artifacts in result: %#v", result.Artifacts)
	}
}

func TestCheckRejectsUnsafePrivacyConfig(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "send_code: false", "send_code: true")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitValidation {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "artifact_contract_validation_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckFailsClosedForDisabledRedaction(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "fail_closed: true", "fail_closed: false")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitRedactionSafety {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "redaction_safety_failed" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckRequiresLocalModelManifest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
}

func TestCheckRejectsIncompleteLocalModelManifest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), `{
  "model_id": "text-embedding-test"
}
`)

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "missing required field") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func TestModelsPullRecordsLocalManifestWithoutNetwork(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	artifactContent := []byte("deterministic local model artifact")
	artifactRel := filepath.ToSlash(filepath.Join(".relia", "models", "artifact.bin"))
	writeFileForTest(t, filepath.Join(tempDir, filepath.FromSlash(artifactRel)), string(artifactContent))
	digest := sha256.Sum256(artifactContent)

	stdout, stderr, code := runForTest(t, []string{
		"--json",
		"models",
		"pull",
		"--model-id", "text-embedding-test",
		"--version", "2026-06-22",
		"--source-url", "https://example.test/model.bin",
		"--license", "Apache-2.0",
		"--digest", fmt.Sprintf("%x", digest),
		"--cache-path", artifactRel,
		"--update-policy", "manual",
		"--rollback-policy", "delete artifact and restore signature embeddings",
	}, false)

	if code != ExitSuccess {
		t.Fatalf("models pull exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Command != "models pull" || result.Status != "pass" {
		t.Fatalf("result = %#v", result)
	}
	manifestPath := filepath.Join(tempDir, ".relia", "models", "manifest.json")
	var manifest map[string]any
	readJSONFileForTest(t, manifestPath, &manifest)
	if manifest["model_id"] != "text-embedding-test" ||
		manifest["version"] != "2026-06-22" ||
		manifest["source_url"] != "https://example.test/model.bin" ||
		manifest["license"] != "Apache-2.0" ||
		manifest["cache_path"] != artifactRel ||
		manifest["update_policy"] != "manual" ||
		manifest["rollback_policy"] == "" ||
		manifest["status"] != "ready" {
		t.Fatalf("manifest = %#v", manifest)
	}
	if commandErr := validateLocalModelManifest(tempDir, yamlScalar{Value: ".relia/models/manifest.json", Line: 1}); commandErr != nil {
		t.Fatalf("manifest did not validate after models pull: %#v", commandErr)
	}
}

func TestModelsPullRejectsCachePathAtManifestPath(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	digest := sha256.Sum256([]byte("manifest collision payload"))

	stdout, stderr, code := runForTest(t, []string{
		"--json",
		"models",
		"pull",
		"--model-id", "text-embedding-test",
		"--version", "2026-06-22",
		"--source-url", "https://example.test/model.bin",
		"--license", "Apache-2.0",
		"--digest", fmt.Sprintf("%x", digest),
		"--cache-path", ".relia/models/manifest.json",
		"--update-policy", "manual",
		"--rollback-policy", "delete artifact and restore signature embeddings",
	}, false)

	if code != ExitUsage {
		t.Fatalf("models pull manifest cache exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "invalid_usage" || !strings.Contains(result.Errors[0].Message, "must not equal") {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".relia", "models", "manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifest path exists after rejected models pull: %v", err)
	}
}

func TestCheckValidatesLocalModelManifestDigest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	artifactContent := []byte("deterministic local model artifact")
	artifactRel := filepath.Join(".relia", "models", "artifact.bin")
	writeFileForTest(t, filepath.Join(tempDir, artifactRel), string(artifactContent))
	digest := sha256.Sum256(artifactContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), fmt.Sprintf(`{
  "model_id": "text-embedding-test",
  "version": "2026-06-22",
  "source_url": "https://example.test/model.bin",
  "license": "Apache-2.0",
  "digest": "%x",
  "cache_path": "%s",
  "update_policy": "manual",
  "rollback_policy": "delete artifact and restore signature embeddings"
}
`, digest, filepath.ToSlash(artifactRel)))

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Status != "pass" {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestCheckRejectsStaleLocalModelManifest(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	artifactContent := []byte("deterministic local model artifact")
	artifactRel := filepath.Join(".relia", "models", "artifact.bin")
	writeFileForTest(t, filepath.Join(tempDir, artifactRel), string(artifactContent))
	digest := sha256.Sum256(artifactContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), fmt.Sprintf(`{
  "model_id": "text-embedding-test",
  "version": "2026-06-22",
  "source_url": "https://example.test/model.bin",
  "license": "Apache-2.0",
  "digest": "%x",
  "cache_path": "%s",
  "update_policy": "manual",
  "rollback_policy": "delete artifact and restore signature embeddings",
  "status": "stale"
}
`, digest, filepath.ToSlash(artifactRel)))

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" || !strings.Contains(result.Errors[0].Message, "stale") {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestCheckRejectsEscapedLocalModelCachePath(t *testing.T) {
	tempDir := setupContractRepo(t)
	t.Chdir(tempDir)
	replaceInFile(t, filepath.Join(tempDir, "relia.yaml"), "embeddings: signature", "embeddings: local")
	artifactContent := []byte("outside model artifact")
	outsideRel := "outside-model.bin"
	writeFileForTest(t, filepath.Join(tempDir, outsideRel), string(artifactContent))
	digest := sha256.Sum256(artifactContent)
	writeFileForTest(t, filepath.Join(tempDir, ".relia", "models", "manifest.json"), fmt.Sprintf(`{
  "model_id": "text-embedding-test",
  "version": "2026-06-22",
  "source_url": "https://example.test/model.bin",
  "license": "Apache-2.0",
  "digest": "%x",
  "cache_path": "../%s",
  "update_policy": "manual",
  "rollback_policy": "delete artifact and restore signature embeddings"
}
`, digest, outsideRel))

	stdout, stderr, code := runForTest(t, []string{"--json", "check"}, false)

	if code != ExitDependency {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	result := decodeResult(t, stdout)
	if result.Errors[0].Type != "dependency_error" {
		t.Fatalf("error type = %q", result.Errors[0].Type)
	}
	if !strings.Contains(result.Errors[0].Message, "inside the repository") {
		t.Fatalf("error message = %q", result.Errors[0].Message)
	}
}

func runForTest(t *testing.T, args []string, stdoutIsTTY bool) (string, string, int) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(args, &stdout, &stderr, stdoutIsTTY)
	return stdout.String(), stderr.String(), code
}

func decodeResult(t *testing.T, output string) CommandResult {
	t.Helper()

	var result CommandResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode command result from %q: %v", output, err)
	}
	return result
}

func decodeBacktestReportFromResult(t *testing.T, result CommandResult) recurrenceReport {
	t.Helper()
	encoded, err := json.Marshal(result.Data["report"])
	if err != nil {
		t.Fatalf("encode nested backtest report: %v", err)
	}
	var report recurrenceReport
	if err := json.Unmarshal(encoded, &report); err != nil {
		t.Fatalf("decode nested backtest report from %s: %v", encoded, err)
	}
	return report
}

func decodeJSONLines(t *testing.T, content string) []map[string]any {
	t.Helper()

	var records []map[string]any
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode JSON line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func loadRuleDocsByKindForTest(t *testing.T, root string) map[string]yamlDocument {
	t.Helper()
	return loadRuleDocsByScalarForTest(t, root, "kind")
}

func loadRuleDocsByStatusForTest(t *testing.T, root string) map[string]yamlDocument {
	t.Helper()
	return loadRuleDocsByScalarForTest(t, root, "status")
}

func findRuleDocByEvidenceForTest(t *testing.T, root string, kind string, experienceIDs []string) yamlDocument {
	t.Helper()
	for _, document := range loadRuleDocsForTest(t, root) {
		if document.Scalars["kind"].Value != kind {
			continue
		}
		if stringSlicesEqual(yamlScalarValuesForTest(document.Lists["evidence.experiences"]), experienceIDs) {
			return document
		}
	}
	t.Fatalf("could not find %s rule with evidence experiences %#v", kind, experienceIDs)
	return yamlDocument{}
}

func distillClusterKeyForTest(kind string, signatureID string, signatureClass string, checkName string, signatureKey string) string {
	return distillClusterKey(experienceRecord{
		Outcome: experienceOutcome{
			Kind: kind,
			Signature: experienceSignature{
				SignatureID: signatureID,
			},
		},
		Metadata: map[string]any{
			"signature": map[string]any{
				"class":      signatureClass,
				"check_name": checkName,
				"key":        signatureKey,
			},
		},
	})
}

func yamlScalarValuesForTest(scalars []yamlScalar) []string {
	values := make([]string, 0, len(scalars))
	for _, scalar := range scalars {
		values = append(values, scalar.Value)
	}
	return values
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assertReportDiagnosticTypes(t *testing.T, diagnostics []reportDiagnostic, wantTypes []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, diagnostic := range diagnostics {
		seen[diagnostic.Type] = true
		if diagnostic.Status == "" || diagnostic.Message == "" || diagnostic.Ref == "" {
			t.Fatalf("diagnostic missing operator-visible details: %#v", diagnostic)
		}
	}
	for _, want := range wantTypes {
		if !seen[want] {
			t.Fatalf("diagnostics missing %q: %#v", want, diagnostics)
		}
	}
}

func loadRuleDocsByScalarForTest(t *testing.T, root string, scalar string) map[string]yamlDocument {
	t.Helper()
	docs := map[string]yamlDocument{}
	for _, document := range loadRuleDocsForTest(t, root) {
		key := document.Scalars[scalar].Value
		if key == "" {
			t.Fatalf("rule missing scalar %s: %#v", scalar, document.Scalars)
		}
		docs[key] = document
	}
	return docs
}

func loadRuleDocsForTest(t *testing.T, root string) []yamlDocument {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "memory", "rules", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("expected generated memory rule YAML files")
	}
	var docs []yamlDocument
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		document := parseRuleDocForTest(t, string(content))
		docs = append(docs, document)
	}
	return docs
}

func readRuleByIDForTest(t *testing.T, root string, ruleID string) string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "memory", "rules", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		document := parseRuleDocForTest(t, string(content))
		if document.Scalars["id"].Value == ruleID {
			return string(content)
		}
	}
	t.Fatalf("could not find generated rule %q", ruleID)
	return ""
}

func parseRuleDocForTest(t *testing.T, content string) yamlDocument {
	t.Helper()
	document, err := parseYAMLDocument(content)
	if err != nil {
		t.Fatalf("parse rule YAML:\n%s\n%v", content, err)
	}
	return document
}

func findRepoRootForTest(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if root, ok := configdoc.FindRepoRoot(wd); ok {
		return root
	}
	t.Fatalf("could not find repo root from %s", wd)
	return ""
}

func setupContractRepo(t *testing.T) string {
	t.Helper()

	sourceRoot := findRepoRootForTest(t)
	tempDir := t.TempDir()
	files := map[string]string{
		"AGENTS.md":              "repo contract\n",
		"WORKFLOW.md":            "workflow contract\n",
		"README.md":              "readme\n",
		"Makefile":               "prepush-full:\n",
		".tool-versions":         "golang 1.26.4\n",
		"go.mod":                 "module github.com/Clyra-AI/relia\n\ngo 1.26.4\n",
		"relia.yaml":             defaultConfigYAML(),
		"docs/product/prd.md":    "prd\n",
		"docs/dev/dev_guides.md": "dev guides\n",
		"docs/architecture/architecture_guides.md": "architecture guides\n",
		"packages/billing/.keep":                   "\n",
		"tests/.keep":                              "\n",
		".github/required-checks.json":             "{}\n",
		".github/workflows/validate.yml":           "name: validate\n",
		".github/workflows/codeql.yml":             "name: codeql\n",
		".factory/factoryd.example.json":           "{}\n",
		".factory/factoryd.autoship.example.json":  "{}\n",
	}
	for rel, content := range files {
		writeFileForTest(t, filepath.Join(tempDir, rel), content)
	}
	for _, rel := range requiredSchemaFiles {
		content, err := os.ReadFile(filepath.Join(sourceRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		writeFileForTest(t, filepath.Join(tempDir, rel), string(content))
	}
	return tempDir
}

func enableProviderForTest(t *testing.T, root string, provider string, model string, baseURL string, credentialEnv string, maxCost string) {
	t.Helper()
	replaceInFile(t, filepath.Join(root, "relia.yaml"), `distill:
  embeddings: signature
  provider: none
  model: ""
  base_url: ""
  credential_env: ""
  max_cost_usd_per_run: 0
  input_cost_usd_per_1k_tokens: 0
  output_cost_usd_per_1k_tokens: 0
  review_required: true`, fmt.Sprintf(`distill:
  embeddings: signature
  provider: %s
  model: %s
  base_url: %s
  credential_env: %s
  max_cost_usd_per_run: %s
  input_cost_usd_per_1k_tokens: 0.001
  output_cost_usd_per_1k_tokens: 0.002
  review_required: true`, provider, model, baseURL, credentialEnv, maxCost))
}

func writeFileForTest(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func replaceInFile(t *testing.T, path string, old string, new string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	next := strings.Replace(string(content), old, new, 1)
	if next == string(content) {
		t.Fatalf("expected to replace %q in %s", old, path)
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		t.Fatal(err)
	}
}
