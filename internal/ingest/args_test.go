package ingest

import "testing"

func TestParseArgsRequiresInput(t *testing.T) {
	_, parseErr := ParseArgs(nil)
	if parseErr == nil {
		t.Fatal("expected missing input error")
	}
	if parseErr.Message != "ingest requires --input <json-or-jsonl> in offline mode" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsAcceptsInputAliases(t *testing.T) {
	for _, flag := range []string{"--input", "-i"} {
		t.Run(flag, func(t *testing.T) {
			options, parseErr := ParseArgs([]string{flag, "experiences.jsonl"})
			if parseErr != nil {
				t.Fatalf("ParseArgs returned error: %v", parseErr)
			}
			if options.InputPath != "experiences.jsonl" {
				t.Fatalf("InputPath = %q, want experiences.jsonl", options.InputPath)
			}
		})
	}
}

func TestParseArgsRejectsMissingInputValue(t *testing.T) {
	_, parseErr := ParseArgs([]string{"-i"})
	if parseErr == nil {
		t.Fatal("expected missing input value error")
	}
	if parseErr.Message != "ingest requires a path after --input" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsUnknownArgument(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--unknown"})
	if parseErr == nil {
		t.Fatal("expected unknown argument error")
	}
	if parseErr.Message != `unknown ingest argument "--unknown"` {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}
