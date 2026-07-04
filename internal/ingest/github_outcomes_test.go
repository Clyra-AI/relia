package ingest

import "testing"

func TestParseGitHubOutcomeEventsTranslatesStructuredExport(t *testing.T) {
	events, ingestErr := ParseGitHubOutcomeEvents([]byte(`{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 301,
      "head_sha": "abc301",
      "html_url": "https://github.com/acme/billing-service/pull/301",
      "merged_at": "2026-06-01T12:00:00Z",
      "labels": [{"name": "agent-authored"}],
      "author": {"login": "acme-agent"},
      "files": [{"filename": "packages/billing/invoice.py"}],
      "check_runs": [{"name": "validate", "conclusion": "success"}]
    },
    {
      "number": 302,
      "head_sha": "abc302",
      "html_url": "https://github.com/acme/billing-service/pull/302",
      "merged_at": "2026-06-02T12:00:00Z",
      "labels": ["agent-authored"],
      "files": ["packages/billing/tax.py"],
      "check_runs": [
        {
          "name": "validate",
          "conclusion": "failure",
          "completed_at": "2026-06-02T12:10:00Z",
          "html_url": "https://github.com/acme/billing-service/actions/runs/302",
          "summary": "unit test failed",
          "paths": ["packages/billing/tax.py"]
        }
      ],
      "reverts": [
        {
          "created_at": "2026-06-03T12:00:00Z",
          "commit_sha": "def302",
          "commit_url": "https://github.com/acme/billing-service/commit/def302",
          "message": "Revert tax change",
          "paths": ["packages/billing/tax.py"]
        }
      ],
      "review_corrections": [
        {
          "marked": true,
          "resolved_at": "2026-06-04T12:00:00Z",
          "html_url": "https://github.com/acme/billing-service/pull/302",
          "path": "packages/billing/tax.py",
          "message": "Fix review finding"
        },
        {
          "marked": false,
          "resolved_at": "2026-06-04T12:30:00Z",
          "html_url": "https://github.com/acme/billing-service/pull/302",
          "path": "packages/billing/ignored.py"
        }
      ],
      "marked_review_corrections": [
        {
          "resolved_at": "2026-06-04T13:00:00Z",
          "html_url": "https://github.com/acme/billing-service/pull/302#discussion_r123",
          "path": "packages/billing/tax.py",
          "message": "Follow-up marked correction"
        }
      ]
    }
  ]
}`), "github-outcomes.json")

	if ingestErr != nil {
		t.Fatalf("ParseGitHubOutcomeEvents returned error: %v", ingestErr)
	}
	if len(events) != 5 {
		t.Fatalf("events = %d, want 5: %#v", len(events), events)
	}
	wantKinds := []string{"merged_clean", "ci_failure", "revert", "review_correction", "review_correction"}
	for index, want := range wantKinds {
		if got := stringFromAny(events[index]["outcome_kind"]); got != want {
			t.Fatalf("event %d outcome_kind = %q, want %q", index, got, want)
		}
		if got := stringFromAny(events[index]["extraction_confidence"]); got != "structured" {
			t.Fatalf("event %d extraction_confidence = %q", index, got)
		}
	}
	if got := stringFromAny(events[1]["signature_key"]); got != "packages/billing/tax.py" {
		t.Fatalf("check-run signature_key = %q, want changed path", got)
	}
	labels := events[0]["labels"].([]string)
	if len(labels) != 1 || labels[0] != "agent-authored" {
		t.Fatalf("labels = %#v, want GitHub object label name", labels)
	}
	if got := stringFromAny(events[2]["commit"]); got != "abc302" {
		t.Fatalf("revert action commit = %q, want original PR head", got)
	}
	metadata := events[2]["metadata"].(map[string]any)
	if got := stringFromAny(metadata["github_source_commit"]); got != "def302" {
		t.Fatalf("revert metadata github_source_commit = %q, want def302", got)
	}
	provenanceURLs := events[4]["provenance_urls"].([]string)
	if len(provenanceURLs) != 1 || provenanceURLs[0] != "https://github.com/acme/billing-service/pull/302" {
		t.Fatalf("fragment provenance_urls = %#v", provenanceURLs)
	}
}

func TestParseGitHubOutcomeEventsRequiresStructuredInputs(t *testing.T) {
	_, ingestErr := ParseGitHubOutcomeEvents([]byte(`{"repo":"acme/billing-service","pull_requests":[{"number":0}]}`), "github-outcomes.json")
	if ingestErr == nil || ingestErr.Kind != ErrorProvenance {
		t.Fatalf("error = %#v, want provenance error", ingestErr)
	}
}

func TestParseGitHubOutcomeEventsAllowsPerPullRepos(t *testing.T) {
	events, ingestErr := ParseGitHubOutcomeEvents([]byte(`{
  "pull_requests": [
    {
      "repo_owner": "acme",
      "repo_name": "billing-service",
      "number": 401,
      "head_sha": "abc401",
      "html_url": "https://github.com/acme/billing-service/pull/401",
      "merged_at": "2026-06-05T12:00:00Z",
      "labels": [{"name": "agent-authored"}],
      "files": [{"filename": "packages/billing/invoice.py"}]
    }
  ]
}`), "github-outcomes.json")

	if ingestErr != nil {
		t.Fatalf("ParseGitHubOutcomeEvents returned error: %v", ingestErr)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1: %#v", len(events), events)
	}
	repo := events[0]["repo"].(map[string]any)
	if repo["owner"] != "acme" || repo["name"] != "billing-service" {
		t.Fatalf("repo = %#v", repo)
	}
}

func TestParseGitHubOutcomeEventsPrefersHeadSHAOverMergeCommit(t *testing.T) {
	events, ingestErr := ParseGitHubOutcomeEvents([]byte(`{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 402,
      "head": {"sha": "head402"},
      "merge_commit_sha": "merge402",
      "html_url": "https://github.com/acme/billing-service/pull/402",
      "merged_at": "2026-06-06T12:00:00Z",
      "labels": [{"name": "agent-authored"}],
      "files": [{"filename": "packages/billing/invoice.py"}]
    }
  ]
}`), "github-outcomes.json")

	if ingestErr != nil {
		t.Fatalf("ParseGitHubOutcomeEvents returned error: %v", ingestErr)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1: %#v", len(events), events)
	}
	if got := stringFromAny(events[0]["commit"]); got != "head402" {
		t.Fatalf("commit = %q, want PR head SHA", got)
	}
}

func TestParseGitHubOutcomeEventsPreservesCoauthorTrailers(t *testing.T) {
	events, ingestErr := ParseGitHubOutcomeEvents([]byte(`{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 402,
      "head_sha": "abc402",
      "html_url": "https://github.com/acme/billing-service/pull/402",
      "merged_at": "2026-06-06T12:00:00Z",
      "coauthor_trailers": ["Claude Code"],
      "files": [{"filename": "packages/billing/invoice.py"}]
    }
  ]
}`), "github-outcomes.json")

	if ingestErr != nil {
		t.Fatalf("ParseGitHubOutcomeEvents returned error: %v", ingestErr)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1: %#v", len(events), events)
	}
	coauthors := events[0]["coauthors"].([]string)
	if len(coauthors) != 1 || coauthors[0] != "Claude Code" {
		t.Fatalf("coauthors = %#v, want Claude Code", coauthors)
	}
}

func TestParseGitHubOutcomeEventsPreservesExplicitAttribution(t *testing.T) {
	events, ingestErr := ParseGitHubOutcomeEvents([]byte(`{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 403,
      "head_sha": "abc403",
      "html_url": "https://github.com/acme/billing-service/pull/403",
      "actor_kind": "agent",
      "attribution_method": "manual",
      "attribution_confidence": 0.97,
      "files": [{"filename": "packages/billing/invoice.py"}],
      "check_runs": [
        {
          "name": "validate",
          "conclusion": "failure",
          "completed_at": "2026-06-07T12:00:00Z",
          "html_url": "https://github.com/acme/billing-service/actions/runs/403",
          "summary": "unit test failed"
        }
      ]
    }
  ]
}`), "github-outcomes.json")

	if ingestErr != nil {
		t.Fatalf("ParseGitHubOutcomeEvents returned error: %v", ingestErr)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1: %#v", len(events), events)
	}
	if got := stringFromAny(events[0]["actor_kind"]); got != "agent" {
		t.Fatalf("actor_kind = %q, want agent", got)
	}
	if got := stringFromAny(events[0]["attribution_method"]); got != "manual" {
		t.Fatalf("attribution_method = %q, want manual", got)
	}
	if got, ok := numericValue(events[0]["attribution_confidence"]); !ok || got != 0.97 {
		t.Fatalf("attribution_confidence = %#v, want 0.97", events[0]["attribution_confidence"])
	}
}

func TestParseGitHubOutcomeEventsAllowsOutcomeLevelPaths(t *testing.T) {
	events, ingestErr := ParseGitHubOutcomeEvents([]byte(`{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 404,
      "head_sha": "abc404",
      "html_url": "https://github.com/acme/billing-service/pull/404",
      "check_runs": [
        {
          "name": "validate",
          "conclusion": "failure",
          "completed_at": "2026-06-08T12:00:00Z",
          "html_url": "https://github.com/acme/billing-service/actions/runs/404",
          "flake_discount": 0.25,
          "paths": ["cmd/relia/main.go"]
        }
      ],
      "reverts": [
        {
          "created_at": "2026-06-09T12:00:00Z",
          "commit_sha": "def404",
          "commit_url": "https://github.com/acme/billing-service/commit/def404",
          "flake_discount": 0.5,
          "signature_class": "test_failure",
          "check_name": "pytest-billing",
          "signature_key": "tests/billing/test_invoice.py::test_clock",
          "paths": ["internal/ingest/github_outcomes.go"]
        }
      ],
      "review_corrections": [
        {
          "marked": true,
          "resolved_at": "2026-06-10T12:00:00Z",
          "html_url": "https://github.com/acme/billing-service/pull/404#discussion_r404",
          "flake_discount": 0.75,
          "path": "internal/ingest/github_outcomes_test.go"
        }
      ]
    }
  ]
}`), "github-outcomes.json")

	if ingestErr != nil {
		t.Fatalf("ParseGitHubOutcomeEvents returned error: %v", ingestErr)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3: %#v", len(events), events)
	}
	for index, want := range []string{
		"cmd/relia/main.go",
		"internal/ingest/github_outcomes.go",
		"internal/ingest/github_outcomes_test.go",
	} {
		paths := events[index]["paths"].([]string)
		if len(paths) != 1 || paths[0] != want {
			t.Fatalf("event %d paths = %#v, want %q", index, paths, want)
		}
	}
	for index, want := range []float64{0.25, 0.5, 0.75} {
		if got, ok := numericValue(events[index]["flake_discount"]); !ok || got != want {
			t.Fatalf("event %d flake_discount = %#v, want %.2f", index, events[index]["flake_discount"], want)
		}
	}
	if got := stringFromAny(events[1]["signature_class"]); got != "test_failure" {
		t.Fatalf("revert signature_class = %q, want test_failure", got)
	}
	if got := stringFromAny(events[1]["check_name"]); got != "pytest-billing" {
		t.Fatalf("revert check_name = %q, want pytest-billing", got)
	}
	if got := stringFromAny(events[1]["signature_key"]); got != "tests/billing/test_invoice.py::test_clock" {
		t.Fatalf("revert signature_key = %q, want explicit test key", got)
	}
}

func TestParseGitHubOutcomeEventsRejectsPathlessEmittedOutcome(t *testing.T) {
	_, ingestErr := ParseGitHubOutcomeEvents([]byte(`{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 406,
      "head_sha": "abc406",
      "html_url": "https://github.com/acme/billing-service/pull/406",
      "merged_at": "2026-06-11T12:00:00Z"
    }
  ]
}`), "github-outcomes.json")

	if ingestErr == nil || ingestErr.Kind != ErrorArtifactContract {
		t.Fatalf("error = %#v, want artifact contract error", ingestErr)
	}
}

func TestParseGitHubOutcomeEventsPrefersCanonicalPRURL(t *testing.T) {
	events, ingestErr := ParseGitHubOutcomeEvents([]byte(`{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 407,
      "head_sha": "abc407",
      "url": "https://api.github.com/repos/acme/billing-service/pulls/407",
      "pr_url": "https://github.com/acme/billing-service/pull/407",
      "merged_at": "2026-06-12T12:00:00Z",
      "files": [{"filename": "packages/billing/invoice.py"}]
    }
  ]
}`), "github-outcomes.json")

	if ingestErr != nil {
		t.Fatalf("ParseGitHubOutcomeEvents returned error: %v", ingestErr)
	}
	provenanceURLs := events[0]["provenance_urls"].([]string)
	if len(provenanceURLs) != 1 || provenanceURLs[0] != "https://github.com/acme/billing-service/pull/407" {
		t.Fatalf("provenance_urls = %#v, want canonical PR URL", provenanceURLs)
	}
}

func TestParseGitHubOutcomeEventsPreservesCheckRunSignatureClass(t *testing.T) {
	events, ingestErr := ParseGitHubOutcomeEvents([]byte(`{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 404,
      "head_sha": "abc404",
      "html_url": "https://github.com/acme/billing-service/pull/404",
      "labels": [{"name": "agent-authored"}],
      "files": [{"filename": "cmd/relia/main.go"}],
      "check_runs": [
        {
          "name": "validate",
          "check_name": "golangci-lint",
          "conclusion": "failure",
          "completed_at": "2026-06-08T12:00:00Z",
          "html_url": "https://github.com/acme/billing-service/actions/runs/404",
          "signature_class": "lint_failure",
          "signature_key": "cmd/relia/main.go::lint"
        }
      ]
    }
  ]
}`), "github-outcomes.json")

	if ingestErr != nil {
		t.Fatalf("ParseGitHubOutcomeEvents returned error: %v", ingestErr)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1: %#v", len(events), events)
	}
	if got := stringFromAny(events[0]["signature_class"]); got != "lint_failure" {
		t.Fatalf("signature_class = %q, want lint_failure", got)
	}
	if got := stringFromAny(events[0]["check_name"]); got != "golangci-lint" {
		t.Fatalf("check_name = %q, want golangci-lint", got)
	}
	if got := stringFromAny(events[0]["signature_key"]); got != "cmd/relia/main.go::lint" {
		t.Fatalf("signature_key = %q, want explicit lint key", got)
	}
}

func TestParseGitHubOutcomeEventsPreservesCleanMergeSignatureMetadata(t *testing.T) {
	events, ingestErr := ParseGitHubOutcomeEvents([]byte(`{
  "repo": {"provider": "github", "owner": "acme", "name": "billing-service"},
  "pull_requests": [
    {
      "number": 405,
      "head_sha": "abc405",
      "html_url": "https://github.com/acme/billing-service/pull/405",
      "merged_at": "2026-06-09T12:00:00Z",
      "labels": [{"name": "agent-authored"}],
      "files": [{"filename": "packages/billing/invoice.py"}],
      "signature_id": "sig_billing_clock",
      "signature_class": "test_failure",
      "check_name": "pytest-billing",
      "signature_key": "tests/billing/test_invoice.py::test_clock",
      "message_fingerprint": "sha256:clock-failure",
      "extraction_confidence": "log_parsed_high"
    }
  ]
}`), "github-outcomes.json")

	if ingestErr != nil {
		t.Fatalf("ParseGitHubOutcomeEvents returned error: %v", ingestErr)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1: %#v", len(events), events)
	}
	event := events[0]
	if got := stringFromAny(event["outcome_kind"]); got != "merged_clean" {
		t.Fatalf("outcome_kind = %q, want merged_clean", got)
	}
	for key, want := range map[string]string{
		"signature_id":          "sig_billing_clock",
		"signature_class":       "test_failure",
		"check_name":            "pytest-billing",
		"signature_key":         "tests/billing/test_invoice.py::test_clock",
		"message_fingerprint":   "sha256:clock-failure",
		"extraction_confidence": "log_parsed_high",
	} {
		if got := stringFromAny(event[key]); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}
