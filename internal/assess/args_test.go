package assess

import "testing"

func TestParseArgsRequiresInput(t *testing.T) {
	_, parseErr := ParseArgs(nil)
	if parseErr == nil {
		t.Fatal("expected missing input error")
	}
	if parseErr.Message != "assess requires --input <diff-or-plan> in offline mode" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsAcceptsInputAliases(t *testing.T) {
	for _, flag := range []string{"--input", "--diff", "-i"} {
		t.Run(flag, func(t *testing.T) {
			options, parseErr := ParseArgs([]string{flag, "change.diff"})
			if parseErr != nil {
				t.Fatalf("ParseArgs returned error: %v", parseErr)
			}
			if options.InputPath != "change.diff" {
				t.Fatalf("InputPath = %q, want change.diff", options.InputPath)
			}
			if options.Format != "json" {
				t.Fatalf("Format = %q, want json", options.Format)
			}
			if options.FormatExplicit {
				t.Fatal("FormatExplicit = true, want false")
			}
		})
	}
}

func TestParseArgsTracksExplicitFormat(t *testing.T) {
	options, parseErr := ParseArgs([]string{"--input", "change.diff", "--format", "json"})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if !options.FormatExplicit {
		t.Fatal("FormatExplicit = false, want true")
	}
}

func TestParseArgsRejectsMissingInputValue(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--diff"})
	if parseErr == nil {
		t.Fatal("expected missing input value error")
	}
	if parseErr.Message != "assess requires a path after --diff" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsUnsupportedFormat(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--input", "change.diff", "--format", "text"})
	if parseErr == nil {
		t.Fatal("expected unsupported format error")
	}
	if parseErr.Message != "assess only supports --format json in this task slice" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}
