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
