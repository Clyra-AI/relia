package ingest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFetchGitHubLiveOutcomeExportRequiresExplicitApprovalBeforeRequest(t *testing.T) {
	client := &fakeGitHubLiveClient{}
	_, _, ingestErr := FetchGitHubLiveOutcomeExport(context.Background(), client, GitHubLiveOptions{
		Repo:        Repo{Provider: "github", Owner: "acme", Name: "billing-service"},
		PullNumbers: []int{302},
		TokenEnv:    "RELIA_GITHUB_TOKEN",
		Token:       "test-token",
		TokenScope:  "read-only",
	})
	if ingestErr == nil || ingestErr.Kind != ErrorCredential {
		t.Fatalf("error = %#v, want credential gate error", ingestErr)
	}
	if len(client.requests) != 0 {
		t.Fatalf("live request was attempted before approval gates: %#v", client.requests)
	}
}

func TestFetchGitHubLiveOutcomeExportBuildsReplayableStructuredOutcomes(t *testing.T) {
	client := newFakeGitHubLiveClient(map[string]fakeGitHubLiveResponse{
		"/repos/acme/billing-service/pulls/302": {
			body: `{
				"number": 302,
				"html_url": "https://github.com/acme/billing-service/pull/302",
				"head": {"sha": "abc302"},
				"merge_commit_sha": "merge302",
				"base": {"ref": "main"},
				"merged_at": "2026-06-02T12:00:00Z",
				"created_at": "2026-06-01T12:00:00Z",
				"updated_at": "2026-06-02T12:00:00Z",
				"user": {"login": "relia-agent"},
				"labels": [{"name": "agent-authored"}]
			}`,
		},
		"/repos/acme/billing-service/pulls/302/files?per_page=100": {
			header: http.Header{"Link": {`<https://api.github.com/repos/acme/billing-service/pulls/302/files?per_page=100&page=2>; rel="next"`}},
			body:   `[{"filename": "packages/billing/tax.py"}]`,
		},
		"/repos/acme/billing-service/pulls/302/files?per_page=100&page=2": {
			body: `[{"filename": "packages/billing/tax_test.py"}]`,
		},
		"/repos/acme/billing-service/pulls/302/commits?per_page=100": {
			body: `[
				{
					"sha": "abc302",
					"commit": {
						"message": "Implement tax change\n\nCo-authored-by: Claude Code <claude@example.invalid>"
					}
				}
			]`,
		},
		"/repos/acme/billing-service/commits/abc302/check-runs?per_page=100": {
			body: `{
				"check_runs": [
					{
						"name": "validate",
						"conclusion": "failure",
						"completed_at": "2026-06-02T12:10:00Z",
						"html_url": "https://github.com/acme/billing-service/actions/runs/302",
						"head_sha": "abc302",
						"output": {"summary": "unit test failed"}
					}
				]
			}`,
		},
		"/repos/acme/billing-service/pulls/302/comments?per_page=100": {
			body: `[
				{
					"body": "relia:review-correction\nFix review finding",
					"path": "packages/billing/tax.py",
					"html_url": "https://github.com/acme/billing-service/pull/302#discussion_r1",
					"updated_at": "2026-06-04T12:00:00Z",
					"commit_id": "abc302"
				}
			]`,
		},
		"/repos/acme/billing-service/issues/302/comments?per_page=100": {
			body: `[]`,
		},
		"/repos/acme/billing-service/commits?per_page=100&sha=main": {
			body: `[
				{
					"sha": "def302",
					"html_url": "https://github.com/acme/billing-service/commit/def302",
					"commit": {
						"message": "Revert tax change\n\nThis reverts commit abc302.",
						"committer": {"date": "2026-06-03T12:00:00Z"}
					}
				}
			]`,
		},
	})

	export, receipt, ingestErr := FetchGitHubLiveOutcomeExport(context.Background(), client, approvedGitHubLiveOptions())
	if ingestErr != nil {
		t.Fatalf("FetchGitHubLiveOutcomeExport returned error: %v", ingestErr)
	}
	if receipt.PullRequestsFetched != 1 || receipt.PagesFetched < 2 {
		t.Fatalf("receipt = %#v, want pull count and paginated fetch evidence", receipt)
	}
	for _, request := range client.requests {
		if request.method != http.MethodGet {
			t.Fatalf("method = %q, want GET for %#v", request.method, request)
		}
		if request.authorization != "Bearer test-token" {
			t.Fatalf("authorization = %q, want bearer token on %#v", request.authorization, request)
		}
	}
	content, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	if strings.Contains(string(content), "test-token") {
		t.Fatalf("live export leaked token: %s", content)
	}
	if ingestErr := ValidateJSONRedactionSafe(content, "github-live-replay.json"); ingestErr != nil {
		t.Fatalf("ValidateJSONRedactionSafe returned error: %v", ingestErr)
	}
	events, ingestErr := ParseGitHubOutcomeEvents(content, "github-live-replay.json")
	if ingestErr != nil {
		t.Fatalf("ParseGitHubOutcomeEvents returned error: %v", ingestErr)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d, want check, revert, and review correction: %#v", len(events), events)
	}
	for index, want := range []string{"ci_failure", "revert", "review_correction"} {
		if got := stringFromAny(events[index]["outcome_kind"]); got != want {
			t.Fatalf("event %d outcome_kind = %q, want %q", index, got, want)
		}
		coauthors := events[index]["coauthors"].([]string)
		if len(coauthors) != 1 || coauthors[0] != "Claude Code" {
			t.Fatalf("event %d coauthors = %#v, want Claude Code", index, coauthors)
		}
		metadata := events[index]["metadata"].(map[string]any)
		if metadata["source_format"] != "github_live_api" {
			t.Fatalf("event %d metadata.source_format = %#v, want github_live_api", index, metadata["source_format"])
		}
	}
}

func TestFetchGitHubLiveOutcomeExportMatchesMergeCommitSHARevert(t *testing.T) {
	client := newFakeGitHubLiveClient(map[string]fakeGitHubLiveResponse{
		"/repos/acme/billing-service/pulls/302": {
			body: `{
				"number": 302,
				"html_url": "https://github.com/acme/billing-service/pull/302",
				"head": {"sha": "abc302"},
				"merge_commit_sha": "merge302",
				"base": {"ref": "main"},
				"merged_at": "2026-06-02T12:00:00Z",
				"created_at": "2026-06-01T12:00:00Z",
				"updated_at": "2026-06-02T12:00:00Z",
				"labels": [{"name": "agent-authored"}]
			}`,
		},
		"/repos/acme/billing-service/pulls/302/files?per_page=100": {
			body: `[{"filename": "packages/billing/tax.py"}]`,
		},
		"/repos/acme/billing-service/pulls/302/commits?per_page=100": {
			body: `[]`,
		},
		"/repos/acme/billing-service/commits/abc302/check-runs?per_page=100": {
			body: `{"check_runs": []}`,
		},
		"/repos/acme/billing-service/pulls/302/comments?per_page=100": {
			body: `[]`,
		},
		"/repos/acme/billing-service/issues/302/comments?per_page=100": {
			body: `[]`,
		},
		"/repos/acme/billing-service/commits?per_page=100&sha=main": {
			body: `[
				{
					"sha": "def302",
					"html_url": "https://github.com/acme/billing-service/commit/def302",
					"commit": {
						"message": "Revert custom merge\n\nThis reverts commit merge302.",
						"committer": {"date": "2026-06-03T12:00:00Z"}
					}
				}
			]`,
		},
	})

	export, _, ingestErr := FetchGitHubLiveOutcomeExport(context.Background(), client, approvedGitHubLiveOptions())
	if ingestErr != nil {
		t.Fatalf("FetchGitHubLiveOutcomeExport returned error: %v", ingestErr)
	}
	content, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	events, ingestErr := ParseGitHubOutcomeEvents(content, "github-live-replay.json")
	if ingestErr != nil {
		t.Fatalf("ParseGitHubOutcomeEvents returned error: %v", ingestErr)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want only revert outcome: %#v", len(events), events)
	}
	if got := stringFromAny(events[0]["outcome_kind"]); got != "revert" {
		t.Fatalf("outcome_kind = %q, want revert", got)
	}
	metadata := events[0]["metadata"].(map[string]any)
	if got := stringFromAny(metadata["github_source_commit"]); got != "def302" {
		t.Fatalf("github_source_commit = %q, want def302", got)
	}
}

func TestFetchGitHubLiveOutcomeExportMapsRateLimit(t *testing.T) {
	client := newFakeGitHubLiveClient(map[string]fakeGitHubLiveResponse{
		"/repos/acme/billing-service/pulls/302": {
			status: http.StatusForbidden,
			header: http.Header{"X-RateLimit-Remaining": {"0"}, "X-RateLimit-Reset": {"1783459200"}},
			body:   `{"message": "API rate limit exceeded"}`,
		},
	})
	_, _, ingestErr := FetchGitHubLiveOutcomeExport(context.Background(), client, approvedGitHubLiveOptions())
	if ingestErr == nil || ingestErr.Kind != ErrorRateLimit {
		t.Fatalf("error = %#v, want rate limit error", ingestErr)
	}
}

func TestFetchGitHubLiveOutcomeExportMapsAuthFailure(t *testing.T) {
	client := newFakeGitHubLiveClient(map[string]fakeGitHubLiveResponse{
		"/repos/acme/billing-service/pulls/302": {
			status: http.StatusUnauthorized,
			body:   `{"message": "Bad credentials"}`,
		},
	})
	_, _, ingestErr := FetchGitHubLiveOutcomeExport(context.Background(), client, approvedGitHubLiveOptions())
	if ingestErr == nil || ingestErr.Kind != ErrorCredential {
		t.Fatalf("error = %#v, want credential error", ingestErr)
	}
}

func TestFetchGitHubLiveOutcomeExportMapsAPIError(t *testing.T) {
	client := newFakeGitHubLiveClient(map[string]fakeGitHubLiveResponse{
		"/repos/acme/billing-service/pulls/302": {
			status: http.StatusInternalServerError,
			body:   `{"message": "server error"}`,
		},
	})
	_, _, ingestErr := FetchGitHubLiveOutcomeExport(context.Background(), client, approvedGitHubLiveOptions())
	if ingestErr == nil || ingestErr.Kind != ErrorGitHubAPI {
		t.Fatalf("error = %#v, want github api error", ingestErr)
	}
}

func TestGitHubLiveCommitRevertsPullRequiresExactPRMarker(t *testing.T) {
	for _, message := range []string{
		`Revert "tax change" (#30)`,
		`Revert tax change via pull/30`,
		`Revert tax change for PR 30`,
		`Revert tax change for pr#30`,
	} {
		if !githubLiveCommitRevertsPull(message, 30, "") {
			t.Fatalf("message %q did not match exact PR 30 marker", message)
		}
	}

	for _, message := range []string{
		`Revert "tax change" (#302)`,
		`Revert tax change via https://github.com/acme/billing-service/pull/302`,
		`Revert tax change for PR 302`,
		`Revert tax change for pr#302`,
	} {
		if githubLiveCommitRevertsPull(message, 30, "") {
			t.Fatalf("message %q matched PR 30, want no prefix match", message)
		}
	}
}

func approvedGitHubLiveOptions() GitHubLiveOptions {
	return GitHubLiveOptions{
		Repo:                Repo{Provider: "github", Owner: "acme", Name: "billing-service"},
		PullNumbers:         []int{302},
		TokenEnv:            "RELIA_GITHUB_TOKEN",
		Token:               "test-token",
		TokenScope:          "read-only",
		NetworkApproved:     true,
		CredentialsApproved: true,
		HumanApproved:       true,
		MaxPages:            4,
	}
}

type fakeGitHubLiveRequest struct {
	method        string
	path          string
	authorization string
}

type fakeGitHubLiveResponse struct {
	status int
	header http.Header
	body   string
}

type fakeGitHubLiveClient struct {
	responses map[string]fakeGitHubLiveResponse
	requests  []fakeGitHubLiveRequest
}

func newFakeGitHubLiveClient(responses map[string]fakeGitHubLiveResponse) *fakeGitHubLiveClient {
	return &fakeGitHubLiveClient{responses: responses}
}

func (c *fakeGitHubLiveClient) Do(request *http.Request) (*http.Response, error) {
	path := request.URL.RequestURI()
	c.requests = append(c.requests, fakeGitHubLiveRequest{
		method:        request.Method,
		path:          path,
		authorization: request.Header.Get("Authorization"),
	})
	response, ok := c.responses[path]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"message":"missing fake response"}`)),
			Request:    request,
		}, nil
	}
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	header := response.header
	if header == nil {
		header = http.Header{}
	}
	canonicalHeader := http.Header{}
	for key, values := range header {
		for _, value := range values {
			canonicalHeader.Add(key, value)
		}
	}
	return &http.Response{
		StatusCode: status,
		Header:     canonicalHeader,
		Body:       io.NopCloser(strings.NewReader(response.body)),
		Request:    request,
	}, nil
}
