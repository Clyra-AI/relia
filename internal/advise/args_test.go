package advise

import "testing"

func TestParseArgsDefaultsAndAliases(t *testing.T) {
	options, parseErr := ParseArgs([]string{"--diff", "changes.diff"})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.InputPath != "changes.diff" || options.Format != "json" {
		t.Fatalf("options = %#v, want default json input", options)
	}
	if options.StatePath != ".relia/reports/advisory-state.json" || options.CommentPath != ".relia/reports/advisory-comment.md" {
		t.Fatalf("default output paths = %#v", options)
	}
}

func TestParseArgsAcceptsCustomPaths(t *testing.T) {
	options, parseErr := ParseArgs([]string{
		"-i", "changes.diff",
		"--state", ".relia/reports/custom-state.json",
		"--comment", ".relia/reports/custom-comment.md",
	})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.StatePath != ".relia/reports/custom-state.json" || options.CommentPath != ".relia/reports/custom-comment.md" {
		t.Fatalf("custom output paths = %#v", options)
	}
}

func TestParseArgsRejectsMissingInput(t *testing.T) {
	_, parseErr := ParseArgs(nil)
	if parseErr == nil || parseErr.Message != "advise requires --input <diff> in offline mode" {
		t.Fatalf("parseErr = %v, want missing input error", parseErr)
	}
}

func TestParseArgsRejectsMissingValues(t *testing.T) {
	for _, args := range [][]string{
		{"--input"},
		{"--format"},
		{"--state"},
		{"--comment"},
	} {
		_, parseErr := ParseArgs(args)
		if parseErr == nil {
			t.Fatalf("ParseArgs(%#v) returned nil error", args)
		}
	}
}

func TestParseArgsRejectsUnsupportedFormatAndUnsafePaths(t *testing.T) {
	for _, args := range [][]string{
		{"--input", "changes.diff", "--format", "text"},
		{"--input", "changes.diff", "--state", "../state.json"},
		{"--input", "changes.diff", "--comment", "/tmp/comment.md"},
	} {
		_, parseErr := ParseArgs(args)
		if parseErr == nil {
			t.Fatalf("ParseArgs(%#v) returned nil error", args)
		}
	}
}
