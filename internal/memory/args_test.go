package memory

import "testing"

func TestParseArgsDefaults(t *testing.T) {
	options, parseErr := ParseArgs(nil)
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Format != "json" {
		t.Fatalf("Format = %q, want json", options.Format)
	}
	if options.OutputPath != "memory/MEMORY.md" {
		t.Fatalf("OutputPath = %q, want memory/MEMORY.md", options.OutputPath)
	}
}

func TestParseArgsCustomOutput(t *testing.T) {
	options, parseErr := ParseArgs([]string{"--format", "json", "--output", "memory/custom.md"})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.OutputPath != "memory/custom.md" {
		t.Fatalf("OutputPath = %q, want memory/custom.md", options.OutputPath)
	}
}

func TestParseArgsRejectsUnsupportedFormat(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--format", "text"})
	if parseErr == nil {
		t.Fatal("expected unsupported format error")
	}
	if parseErr.Message != "memory only supports --format json in this task slice" {
		t.Fatalf("message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsInvalidOutputPath(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--output", "../MEMORY.md"})
	if parseErr == nil {
		t.Fatal("expected invalid output path error")
	}
	if parseErr.Message != "memory --output must be a repo-relative path" {
		t.Fatalf("message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsMissingOutputValue(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--output"})
	if parseErr == nil {
		t.Fatal("expected missing output value error")
	}
	if parseErr.Message != "memory requires a repo-relative path after --output" {
		t.Fatalf("message = %q", parseErr.Message)
	}
}
