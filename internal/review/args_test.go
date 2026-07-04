package review

import "testing"

func TestParseArgsDefaultsToApproveLabel(t *testing.T) {
	options, parseErr := ParseArgs([]string{"--rule", "avoid-demo"})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Action != "label" {
		t.Fatalf("Action = %q, want label", options.Action)
	}
	if options.Label != "accepted" {
		t.Fatalf("Label = %q, want accepted", options.Label)
	}
	if options.Rule != "avoid-demo" {
		t.Fatalf("Rule = %q, want avoid-demo", options.Rule)
	}
}

func TestParseArgsApproveAction(t *testing.T) {
	options, parseErr := ParseArgs([]string{"approve", "--rule", "memory/rules/avoid.yaml"})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Action != "approve" || options.Label != "accepted" {
		t.Fatalf("unexpected options: %+v", options)
	}
}

func TestParseArgsEditAction(t *testing.T) {
	options, parseErr := ParseArgs([]string{
		"edit",
		"--rule", "avoid-demo",
		"--statement", "Prefer the reviewed fix.",
		"--scope-path", "cmd/relia/main.go",
	})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Action != "edit" || options.Label != "suggested" || options.Statement != "Prefer the reviewed fix." {
		t.Fatalf("unexpected options: %+v", options)
	}
	if len(options.ScopePaths) != 1 || options.ScopePaths[0] != "cmd/relia/main.go" {
		t.Fatalf("ScopePaths = %#v, want cmd/relia/main.go", options.ScopePaths)
	}
}

func TestParseArgsRejectAction(t *testing.T) {
	options, parseErr := ParseArgs([]string{"reject", "--rule", "avoid-demo", "--reason", "not applicable"})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Action != "reject" || options.Label != "needs_user_input" || options.Reason != "not applicable" {
		t.Fatalf("unexpected options: %+v", options)
	}
}

func TestParseArgsMergeAction(t *testing.T) {
	options, parseErr := ParseArgs([]string{
		"merge",
		"--rule", "duplicate-rule",
		"--into", "canonical-rule",
		"--reason", "covered by narrower canonical rule",
		"--reviewed-by", "maintainer",
	})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Action != "merge" ||
		options.Label != "needs_user_input" ||
		options.MergeInto != "canonical-rule" ||
		options.ReviewedBy != "maintainer" {
		t.Fatalf("unexpected options: %+v", options)
	}
}

func TestParseArgsRejectsEditInputWithoutEditAction(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--rule", "avoid-demo", "--statement", "new text"})
	if parseErr == nil {
		t.Fatal("expected edit action error")
	}
	if parseErr.Message != "review --statement and --scope-path require review edit" {
		t.Fatalf("message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsIntoWithoutMergeAction(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--rule", "avoid-demo", "--into", "canonical"})
	if parseErr == nil {
		t.Fatal("expected merge action error")
	}
	if parseErr.Message != "review --into requires review merge" {
		t.Fatalf("message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsAcceptedEdit(t *testing.T) {
	_, parseErr := ParseArgs([]string{"edit", "--rule", "avoid-demo", "--statement", "new text", "--label", "accepted"})
	if parseErr == nil {
		t.Fatal("expected accepted edit error")
	}
	if parseErr.Message != "review edit keeps a rule candidate; run review approve after editing" {
		t.Fatalf("message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsInvalidScopePath(t *testing.T) {
	_, parseErr := ParseArgs([]string{"edit", "--rule", "avoid-demo", "--scope-path", "../outside"})
	if parseErr == nil {
		t.Fatal("expected invalid scope path error")
	}
	if parseErr.Message != "review --scope-path must be repo-relative" {
		t.Fatalf("message = %q", parseErr.Message)
	}
}
