package distill

import "testing"

func TestParseArgsDefaults(t *testing.T) {
	options, parseErr := ParseArgs(nil)
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Format != "json" {
		t.Fatalf("Format = %q, want json", options.Format)
	}
	if options.RuleDir != "memory/rules" {
		t.Fatalf("RuleDir = %q, want memory/rules", options.RuleDir)
	}
	if options.HalfLifeDays != 90 {
		t.Fatalf("HalfLifeDays = %d, want 90", options.HalfLifeDays)
	}
}

func TestParseArgsAllOptions(t *testing.T) {
	options, parseErr := ParseArgs([]string{
		"--format", "json",
		"--input", "events.jsonl",
		"--rule-dir", "memory/custom",
		"--half-life-days", "30",
		"--embeddings", "provider",
	})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.InputPath != "events.jsonl" || options.RuleDir != "memory/custom" || options.HalfLifeDays != 30 || options.Embeddings != "provider" {
		t.Fatalf("unexpected options: %+v", options)
	}
}

func TestParseArgsRejectsInvalidRuleDir(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--rule-dir", "../memory"})
	if parseErr == nil {
		t.Fatal("expected invalid rule-dir error")
	}
	if parseErr.Message != "distill --rule-dir must be a repo-relative path" {
		t.Fatalf("message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsInvalidEmbeddingMode(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--embeddings", "remote"})
	if parseErr == nil {
		t.Fatal("expected invalid embeddings error")
	}
	if parseErr.Message != "distill --embeddings must be signature, local, or provider" {
		t.Fatalf("message = %q", parseErr.Message)
	}
}
