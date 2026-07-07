package serve

import "testing"

func TestParseArgsDefaults(t *testing.T) {
	options, parseErr := ParseArgs(nil)
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Format != "json" {
		t.Fatalf("Format = %q, want json", options.Format)
	}
}

func TestParseArgsAcceptsExplicitJSONFormat(t *testing.T) {
	options, parseErr := ParseArgs([]string{"--format", "json"})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Format != "json" {
		t.Fatalf("Format = %q, want json", options.Format)
	}
}

func TestParseArgsAcceptsRecallToolContextAndPaths(t *testing.T) {
	options, parseErr := ParseArgs([]string{"--tool", "recall", "--context", "edit packages/billing/invoice.py", "--paths", "packages/billing/invoice.py, tests/billing/test_invoice.py"})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Tool != "recall" || options.Context == "" {
		t.Fatalf("options = %#v, want recall context", options)
	}
	if len(options.Paths) != 2 || options.Paths[0] != "packages/billing/invoice.py" || options.Paths[1] != "tests/billing/test_invoice.py" {
		t.Fatalf("Paths = %#v", options.Paths)
	}
}

func TestParseArgsAcceptsAssessToolInput(t *testing.T) {
	options, parseErr := ParseArgs([]string{"--tool", "assess", "--input", "change.diff"})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Tool != "assess" || options.InputPath != "change.diff" {
		t.Fatalf("options = %#v, want assess input", options)
	}
}

func TestParseArgsRejectsRecallWithoutQuery(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--tool", "recall"})
	if parseErr == nil {
		t.Fatal("expected recall query error")
	}
	if parseErr.Message != "serve recall requires --context or --paths" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsCoverageWithoutPaths(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--tool", "coverage"})
	if parseErr == nil {
		t.Fatal("expected coverage paths error")
	}
	if parseErr.Message != "serve coverage requires --paths" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsNonRepoRelativePaths(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--tool", "coverage", "--paths", "../secrets.txt"})
	if parseErr == nil {
		t.Fatal("expected repo-relative path error")
	}
	if parseErr.Message != "serve --paths values must be repo-relative" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsUnknownTool(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--tool", "learn", "--context", "packages/billing"})
	if parseErr == nil {
		t.Fatal("expected unknown tool error")
	}
	if parseErr.Message != "serve --tool must be one of recall, assess, or coverage" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsMissingFormatValue(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--format"})
	if parseErr == nil {
		t.Fatal("expected missing format error")
	}
	if parseErr.Kind != ErrorKindUsage {
		t.Fatalf("Kind = %q, want usage", parseErr.Kind)
	}
	if parseErr.Message != "serve requires a value after --format" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsUnsupportedFormat(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--format", "text"})
	if parseErr == nil {
		t.Fatal("expected unsupported format error")
	}
	if parseErr.Kind != ErrorKindUsage {
		t.Fatalf("Kind = %q, want usage", parseErr.Kind)
	}
	if parseErr.Message != "serve only supports --format json in this task slice" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsHostedTransportAsDependency(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--hosted"})
	if parseErr == nil {
		t.Fatal("expected hosted dependency error")
	}
	if parseErr.Kind != ErrorKindDependency {
		t.Fatalf("Kind = %q, want dependency", parseErr.Kind)
	}
	if parseErr.Reference != "docs/product/prd.md#serve-and-advise" {
		t.Fatalf("Reference = %q", parseErr.Reference)
	}
}
