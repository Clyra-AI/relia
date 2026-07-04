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
