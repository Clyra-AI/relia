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

func TestParseArgsAcceptsGitHubOutcomesMode(t *testing.T) {
	options, parseErr := ParseArgs([]string{"--github-outcomes", "--input", "github-outcomes.json"})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if !options.GitHubOutcomes {
		t.Fatal("expected GitHubOutcomes mode")
	}
	if options.InputPath != "github-outcomes.json" {
		t.Fatalf("InputPath = %q, want github-outcomes.json", options.InputPath)
	}
}

func TestParseArgsAcceptsGitHubLiveMode(t *testing.T) {
	options, parseErr := ParseArgs([]string{
		"--github-live",
		"--repo", "acme/billing-service",
		"--pr", "301",
		"--pr", "302",
		"--github-token-env", "RELIA_GITHUB_TOKEN",
		"--github-token-scope", "read-only",
		"--allow-network",
		"--allow-credentials",
		"--human-approved",
	})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if !options.GitHubLive {
		t.Fatal("expected GitHubLive mode")
	}
	if options.GitHubRepo != "acme/billing-service" {
		t.Fatalf("GitHubRepo = %q, want acme/billing-service", options.GitHubRepo)
	}
	if len(options.GitHubPulls) != 2 || options.GitHubPulls[0] != 301 || options.GitHubPulls[1] != 302 {
		t.Fatalf("GitHubPulls = %#v, want [301 302]", options.GitHubPulls)
	}
	if options.GitHubTokenEnv != "RELIA_GITHUB_TOKEN" {
		t.Fatalf("GitHubTokenEnv = %q", options.GitHubTokenEnv)
	}
	if options.GitHubTokenScope != "read-only" {
		t.Fatalf("GitHubTokenScope = %q", options.GitHubTokenScope)
	}
	if !options.AllowNetwork || !options.AllowCredentials || !options.HumanApproved {
		t.Fatalf("approval gates = network:%v credentials:%v human:%v", options.AllowNetwork, options.AllowCredentials, options.HumanApproved)
	}
}

func TestParseArgsRejectsGitHubLiveReplayMix(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--github-live", "--github-outcomes", "--input", "github-outcomes.json"})
	if parseErr == nil {
		t.Fatal("expected github live/replay mode conflict")
	}
	if parseErr.Message != "ingest --github-live cannot be combined with --github-outcomes or --input; use --github-outcomes --input <path> for offline replay" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsGitHubLiveArgumentsWithoutGitHubLiveMode(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--input", "outcomes.json", "--repo", "acme/billing-service"})
	if parseErr == nil {
		t.Fatal("expected github live flag mode error")
	}
	if parseErr.Message != "github live arguments require --github-live" {
		t.Fatalf("Message = %q", parseErr.Message)
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
