package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	githubLiveAPIBaseURL = "https://api.github.com"
	githubLiveAPIHost    = "api.github.com"
	githubLiveSource     = "github_live_api"
	githubLiveMaxPages   = 10
	githubTokenEnvRef    = "relia ingest --github-token-env"
)

type GitHubLiveOptions struct {
	Repo                Repo
	PullNumbers         []int
	TokenEnv            string
	Token               string
	TokenScope          string
	NetworkApproved     bool
	CredentialsApproved bool
	HumanApproved       bool
	MaxPages            int
	UserAgent           string
}

type GitHubLiveReceipt struct {
	SourceFormat        string `json:"source_format"`
	APIHost             string `json:"api_host"`
	TokenEnv            string `json:"token_env"`
	TokenScope          string `json:"token_scope"`
	ReadOnly            bool   `json:"read_only"`
	PullRequestsFetched int    `json:"pull_requests_fetched"`
	RequestsMade        int    `json:"requests_made"`
	PagesFetched        int    `json:"pages_fetched"`
}

type GitHubHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

func ParseGitHubRepoSlug(value string) (Repo, *Error) {
	owner, name, ok := strings.Cut(strings.TrimSpace(value), "/")
	if !ok || strings.Contains(strings.TrimSpace(name), "/") {
		return Repo{}, artifactContractError("github live repo must use owner/name", "relia ingest --github-live --repo")
	}
	repo := Repo{Provider: "github", Owner: strings.TrimSpace(owner), Name: strings.TrimSpace(name)}
	if !validGitHubPathSegment(repo.Owner) || !validGitHubPathSegment(repo.Name) {
		return Repo{}, artifactContractError("github live repo owner and name must be non-empty GitHub path segments", "relia ingest --github-live --repo")
	}
	return repo, nil
}

func FetchGitHubLiveOutcomeExport(ctx context.Context, client GitHubHTTPClient, options GitHubLiveOptions) (map[string]any, GitHubLiveReceipt, *Error) {
	normalized, receipt, commandErr := normalizeGitHubLiveOptions(options)
	if commandErr != nil {
		return nil, receipt, commandErr
	}
	if client == nil {
		client = http.DefaultClient
	}
	fetcher := githubLiveFetcher{
		ctx:     ctx,
		client:  client,
		options: normalized,
		receipt: receipt,
	}
	pulls := make([]any, 0, len(normalized.PullNumbers))
	for _, number := range normalized.PullNumbers {
		pull, commandErr := fetcher.pull(number)
		if commandErr != nil {
			return nil, fetcher.receipt, commandErr
		}
		pulls = append(pulls, pull)
	}
	fetcher.receipt.PullRequestsFetched = len(pulls)
	export := map[string]any{
		"object_type":    "relia.github_outcome_export",
		"schema_version": "1.0",
		"source_format":  githubLiveSource,
		"repo": map[string]any{
			"provider": "github",
			"owner":    normalized.Repo.Owner,
			"name":     normalized.Repo.Name,
		},
		"pull_requests": pulls,
		"metadata": map[string]any{
			"source_format": githubLiveSource,
			"api_host":      githubLiveAPIHost,
			"read_only":     true,
			"token_env":     normalized.TokenEnv,
			"token_scope":   normalized.TokenScope,
		},
	}
	return export, fetcher.receipt, nil
}

type githubLiveFetcher struct {
	ctx     context.Context
	client  GitHubHTTPClient
	options GitHubLiveOptions
	receipt GitHubLiveReceipt
}

func normalizeGitHubLiveOptions(options GitHubLiveOptions) (GitHubLiveOptions, GitHubLiveReceipt, *Error) {
	receipt := GitHubLiveReceipt{
		SourceFormat: githubLiveSource,
		APIHost:      githubLiveAPIHost,
		TokenEnv:     strings.TrimSpace(options.TokenEnv),
		TokenScope:   strings.TrimSpace(options.TokenScope),
		ReadOnly:     true,
	}
	if !options.HumanApproved {
		return options, receipt, credentialRequiredError("live GitHub API intake requires explicit human approval via --human-approved", "docs/architecture/architecture_guides.md#trust-mode-posture")
	}
	if !options.NetworkApproved {
		return options, receipt, credentialRequiredError("live GitHub API intake requires task-scoped network approval via --allow-network for api.github.com", "docs/architecture/architecture_guides.md#trust-mode-posture")
	}
	if !options.CredentialsApproved {
		return options, receipt, credentialRequiredError("live GitHub API intake requires task-scoped credential approval via --allow-credentials", "docs/architecture/architecture_guides.md#trust-mode-posture")
	}
	options.TokenEnv = strings.TrimSpace(options.TokenEnv)
	if options.TokenEnv == "" {
		return options, receipt, credentialRequiredError("live GitHub API intake requires explicit --github-token-env and never reads ambient credentials", "docs/architecture/architecture_guides.md#trust-mode-posture")
	}
	if !validEnvironmentVariableName(options.TokenEnv) {
		return options, receipt, credentialRequiredError("live GitHub API token environment must be an environment variable name", githubTokenEnvRef)
	}
	options.Token = strings.TrimSpace(options.Token)
	if options.Token == "" {
		options.Token = strings.TrimSpace(os.Getenv(options.TokenEnv))
	}
	if options.Token == "" {
		return options, receipt, credentialRequiredError("live GitHub API token environment is unset or empty", githubTokenEnvRef)
	}
	options.TokenScope = strings.TrimSpace(options.TokenScope)
	if options.TokenScope != "read-only" {
		return options, receipt, credentialRequiredError("live GitHub API token scope must be declared as read-only", "relia ingest --github-token-scope")
	}
	if options.Repo.Provider == "" {
		options.Repo.Provider = "github"
	}
	if options.Repo.Provider != "github" ||
		!validGitHubPathSegment(options.Repo.Owner) ||
		!validGitHubPathSegment(options.Repo.Name) {
		return options, receipt, artifactContractError("github live repo must include a valid github owner/name", "relia ingest --github-live --repo")
	}
	if len(options.PullNumbers) == 0 {
		return options, receipt, artifactContractError("github live intake requires at least one pull request number", "relia ingest --github-live --pr")
	}
	for _, number := range options.PullNumbers {
		if number < 1 {
			return options, receipt, artifactContractError("github live pull request numbers must be positive integers", "relia ingest --github-live --pr")
		}
	}
	if options.MaxPages <= 0 {
		options.MaxPages = githubLiveMaxPages
	}
	if strings.TrimSpace(options.UserAgent) == "" {
		options.UserAgent = "relia"
	}
	return options, receipt, nil
}

func (f *githubLiveFetcher) pull(number int) (map[string]any, *Error) {
	pullPath := fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(f.options.Repo.Owner), url.PathEscape(f.options.Repo.Name), number)
	rawPull, commandErr := f.getObject(pullPath, "pull_request")
	if commandErr != nil {
		return nil, commandErr
	}
	headSHA := githubString(rawPull, "head.sha", "head_sha")
	mergeCommitSHA := githubString(rawPull, "merge_commit_sha")
	baseRef := githubString(rawPull, "base.ref", "base_ref")
	mergedAt := githubString(rawPull, "merged_at")
	files, commandErr := f.getArray(pullPath+"/files?per_page=100", "", "pull_request_files")
	if commandErr != nil {
		return nil, commandErr
	}
	prCommits, commandErr := f.getArray(pullPath+"/commits?per_page=100", "", "pull_commits")
	if commandErr != nil {
		return nil, commandErr
	}
	checkRuns := []map[string]any{}
	if headSHA != "" {
		checkPath := fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs?filter=all&per_page=100", url.PathEscape(f.options.Repo.Owner), url.PathEscape(f.options.Repo.Name), url.PathEscape(headSHA))
		checkRuns, commandErr = f.getArray(checkPath, "check_runs", "check_runs")
		if commandErr != nil {
			return nil, commandErr
		}
	}
	reviewComments, commandErr := f.getArray(pullPath+"/comments?per_page=100", "", "pull_review_comments")
	if commandErr != nil {
		return nil, commandErr
	}
	issueCommentsPath := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=100", url.PathEscape(f.options.Repo.Owner), url.PathEscape(f.options.Repo.Name), number)
	issueComments, commandErr := f.getArray(issueCommentsPath, "", "issue_comments")
	if commandErr != nil {
		return nil, commandErr
	}
	commits := []map[string]any{}
	if mergedAt != "" {
		commitsPath := fmt.Sprintf("/repos/%s/%s/commits?per_page=100", url.PathEscape(f.options.Repo.Owner), url.PathEscape(f.options.Repo.Name))
		if baseRef != "" {
			commitsPath += "&sha=" + url.QueryEscape(baseRef)
		}
		commitsPath += "&since=" + url.QueryEscape(mergedAt)
		commits, commandErr = f.getArrayAllowTruncated(commitsPath, "", "commits")
		if commandErr != nil {
			return nil, commandErr
		}
	}

	pull := map[string]any{
		"number":     number,
		"head_sha":   headSHA,
		"html_url":   githubString(rawPull, "html_url"),
		"merged_at":  mergedAt,
		"updated_at": githubString(rawPull, "updated_at"),
		"created_at": githubString(rawPull, "created_at"),
		"files":      sanitizeGitHubLiveFiles(files),
		"check_runs": sanitizeGitHubLiveCheckRuns(checkRuns),
	}
	if mergeCommitSHA != "" {
		pull["merge_commit_sha"] = mergeCommitSHA
	}
	if labels := sanitizeGitHubLiveLabels(githubOutcomeObjects(rawPull, "labels")); len(labels) > 0 {
		pull["labels"] = labels
	}
	if coauthors := sanitizeGitHubLiveCoauthorTrailers(prCommits); len(coauthors) > 0 {
		pull["coauthor_trailers"] = coauthors
	}
	if login := githubString(rawPull, "user.login", "author.login"); login != "" {
		pull["author"] = map[string]any{"login": login}
	}
	if reverts := sanitizeGitHubLiveReverts(commits, number, sanitizeGitHubLivePathList(files), headSHA, mergeCommitSHA); len(reverts) > 0 {
		pull["reverts"] = reverts
	}
	if corrections := sanitizeGitHubLiveReviewCorrections(append(reviewComments, issueComments...)); len(corrections) > 0 {
		pull["marked_review_corrections"] = corrections
	}
	return pull, nil
}

func (f *githubLiveFetcher) getObject(requestPath string, resource string) (map[string]any, *Error) {
	decoded, _, commandErr := f.doJSON(requestPath, resource)
	if commandErr != nil {
		return nil, commandErr
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, githubAPIError("github api "+resource+" response was not a JSON object", resource)
	}
	return object, nil
}

func (f *githubLiveFetcher) getArray(requestPath string, objectKey string, resource string) ([]map[string]any, *Error) {
	return f.getArrayWithPageLimit(requestPath, objectKey, resource, false)
}

func (f *githubLiveFetcher) getArrayAllowTruncated(requestPath string, objectKey string, resource string) ([]map[string]any, *Error) {
	return f.getArrayWithPageLimit(requestPath, objectKey, resource, true)
}

func (f *githubLiveFetcher) getArrayWithPageLimit(requestPath string, objectKey string, resource string, allowTruncated bool) ([]map[string]any, *Error) {
	var result []map[string]any
	nextPath := requestPath
	pages := 0
	for nextPath != "" {
		pages++
		if pages > f.options.MaxPages {
			if allowTruncated {
				return result, nil
			}
			return nil, githubAPIError("github api pagination exceeded configured page limit", resource)
		}
		decoded, header, commandErr := f.doJSON(nextPath, resource)
		if commandErr != nil {
			return nil, commandErr
		}
		values, commandErr := githubLiveArrayFromDecoded(decoded, objectKey, resource)
		if commandErr != nil {
			return nil, commandErr
		}
		result = append(result, values...)
		nextPath = githubLiveNextLink(header.Get("Link"))
	}
	return result, nil
}

func (f *githubLiveFetcher) doJSON(requestPath string, resource string) (any, http.Header, *Error) {
	requestURL, commandErr := githubLiveRequestURL(requestPath)
	if commandErr != nil {
		return nil, nil, commandErr
	}
	request, err := http.NewRequestWithContext(f.ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, nil, internalError("could not build github api request", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", f.options.UserAgent)
	request.Header.Set("Authorization", "Bearer "+f.options.Token)
	f.receipt.RequestsMade++
	response, err := f.client.Do(request)
	if err != nil {
		return nil, nil, githubAPIError("github api request failed", resource)
	}
	defer response.Body.Close()
	f.receipt.PagesFetched++
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, internalError("could not read github api response", err)
	}
	if commandErr := githubLiveStatusError(response, string(body), resource); commandErr != nil {
		return nil, nil, commandErr
	}
	var decoded any
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, response.Header, nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, nil, githubAPIError("github api "+resource+" response was not valid JSON", resource)
	}
	return decoded, response.Header, nil
}

func githubLiveRequestURL(requestPath string) (string, *Error) {
	if strings.HasPrefix(requestPath, "https://") {
		parsed, err := url.Parse(requestPath)
		if err != nil ||
			parsed.Scheme != "https" ||
			!strings.EqualFold(parsed.Host, githubLiveAPIHost) ||
			parsed.User != nil {
			return "", githubAPIError("github api pagination URL was outside the approved allowlist", "pagination")
		}
		return parsed.String(), nil
	}
	if !strings.HasPrefix(requestPath, "/") {
		return "", githubAPIError("github api request path must be absolute", "request")
	}
	return githubLiveAPIBaseURL + requestPath, nil
}

func githubLiveStatusError(response *http.Response, body string, resource string) *Error {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	if ref, ok := githubLiveRateLimitRef(response, body); ok {
		return rateLimitError("github api rate limit reached before live outcome intake completed", ref)
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return credentialRequiredError("github api credentials were rejected for read-only outcome intake", "relia ingest --github-token-env")
	}
	return githubAPIError(fmt.Sprintf("github api %s request failed with status %d", resource, response.StatusCode), resource)
}

func githubLiveRateLimitRef(response *http.Response, body string) (string, bool) {
	isRateLimit := response.StatusCode == http.StatusTooManyRequests
	if response.StatusCode == http.StatusForbidden {
		lowerBody := strings.ToLower(body)
		isRateLimit = strings.TrimSpace(response.Header.Get("X-RateLimit-Remaining")) == "0" ||
			strings.TrimSpace(response.Header.Get("Retry-After")) != "" ||
			strings.Contains(lowerBody, "secondary rate limit") ||
			strings.Contains(lowerBody, "secondary limit")
	}
	if !isRateLimit {
		return "", false
	}
	ref := "github api rate limit"
	if reset := strings.TrimSpace(response.Header.Get("X-RateLimit-Reset")); reset != "" {
		ref += " reset=" + reset
	}
	if retryAfter := strings.TrimSpace(response.Header.Get("Retry-After")); retryAfter != "" {
		ref += " retry-after=" + retryAfter
	}
	return ref, true
}

func githubLiveArrayFromDecoded(decoded any, objectKey string, resource string) ([]map[string]any, *Error) {
	var raw []any
	switch typed := decoded.(type) {
	case []any:
		raw = typed
	case map[string]any:
		if objectKey == "" {
			return nil, githubAPIError("github api "+resource+" response was not a JSON array", resource)
		}
		values, ok := typed[objectKey].([]any)
		if !ok {
			return nil, githubAPIError("github api "+resource+" response missing "+objectKey+" array", resource)
		}
		raw = values
	default:
		return nil, githubAPIError("github api "+resource+" response was not a supported JSON shape", resource)
	}
	result := make([]map[string]any, 0, len(raw))
	for _, value := range raw {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, githubAPIError("github api "+resource+" array contained non-object item", resource)
		}
		result = append(result, object)
	}
	return result, nil
}

func githubLiveNextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start >= 0 && end > start {
			return part[start+1 : end]
		}
	}
	return ""
}

func sanitizeGitHubLiveFiles(files []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(files))
	for _, file := range files {
		if filename := githubString(file, "filename", "path"); filename != "" {
			result = append(result, map[string]any{"filename": filename})
		}
	}
	return result
}

func sanitizeGitHubLivePathList(files []map[string]any) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		if filename := githubString(file, "filename", "path"); filename != "" {
			paths = append(paths, filename)
		}
	}
	return uniqueStrings(paths)
}

func sanitizeGitHubLiveLabels(labels []map[string]any) []string {
	result := make([]string, 0, len(labels))
	for _, label := range labels {
		if name := githubString(label, "name"); name != "" {
			result = append(result, name)
		}
	}
	return uniqueStrings(result)
}

func sanitizeGitHubLiveCheckRuns(checkRuns []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(checkRuns))
	for _, checkRun := range checkRuns {
		sanitized := map[string]any{}
		copyGitHubLiveString(sanitized, checkRun, "name", "name")
		copyGitHubLiveString(sanitized, checkRun, "conclusion", "conclusion")
		copyGitHubLiveString(sanitized, checkRun, "status", "status")
		copyGitHubLiveString(sanitized, checkRun, "completed_at", "completed_at")
		copyGitHubLiveString(sanitized, checkRun, "updated_at", "updated_at")
		copyGitHubLiveString(sanitized, checkRun, "created_at", "created_at")
		copyGitHubLiveString(sanitized, checkRun, "html_url", "html_url")
		copyGitHubLiveString(sanitized, checkRun, "details_url", "details_url")
		copyGitHubLiveString(sanitized, checkRun, "head_sha", "head_sha")
		if summary := githubString(checkRun, "output.summary", "summary"); summary != "" {
			sanitized["summary"] = summary
		}
		result = append(result, sanitized)
	}
	return result
}

func sanitizeGitHubLiveCoauthorTrailers(commits []map[string]any) []string {
	var result []string
	for _, commit := range commits {
		message := githubString(commit, "commit.message", "message")
		for _, line := range strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n") {
			label, value, ok := strings.Cut(line, ":")
			if !ok || !strings.EqualFold(strings.TrimSpace(label), "Co-authored-by") {
				continue
			}
			name := strings.TrimSpace(value)
			if beforeEmail, _, ok := strings.Cut(name, "<"); ok {
				name = strings.TrimSpace(beforeEmail)
			}
			if name != "" {
				result = append(result, name)
			}
		}
	}
	return uniqueStrings(result)
}

func sanitizeGitHubLiveReverts(commits []map[string]any, pr int, paths []string, commitSHAs ...string) []map[string]any {
	var result []map[string]any
	for _, commit := range commits {
		message := githubString(commit, "commit.message", "message")
		if !githubLiveCommitRevertsPull(message, pr, commitSHAs...) {
			continue
		}
		sanitized := map[string]any{
			"commit_sha": githubString(commit, "sha"),
			"commit_url": githubString(commit, "html_url"),
			"message":    firstString(message),
			"paths":      append([]string(nil), paths...),
		}
		if committedAt := githubString(commit, "commit.committer.date", "commit.author.date"); committedAt != "" {
			sanitized["committed_at"] = committedAt
		}
		result = append(result, sanitized)
	}
	return result
}

func sanitizeGitHubLiveReviewCorrections(comments []map[string]any) []map[string]any {
	var result []map[string]any
	for _, comment := range comments {
		body := githubString(comment, "body")
		if !githubLiveReviewCorrectionMarked(body) {
			continue
		}
		sanitized := map[string]any{
			"message": body,
		}
		copyGitHubLiveString(sanitized, comment, "html_url", "html_url")
		copyGitHubLiveString(sanitized, comment, "path", "path")
		copyGitHubLiveString(sanitized, comment, "commit_id", "commit_id")
		if resolvedAt := firstString(githubString(comment, "updated_at"), githubString(comment, "created_at")); resolvedAt != "" {
			sanitized["resolved_at"] = resolvedAt
		}
		result = append(result, sanitized)
	}
	return result
}

func githubLiveCommitRevertsPull(message string, pr int, commitSHAs ...string) bool {
	normalized := strings.ToLower(message)
	if !strings.Contains(normalized, "revert") {
		return false
	}
	for _, commitSHA := range commitSHAs {
		commitSHA = strings.TrimSpace(commitSHA)
		if commitSHA != "" && strings.Contains(normalized, strings.ToLower(commitSHA)) {
			return true
		}
	}
	for _, marker := range []string{
		"#" + strconv.Itoa(pr),
		"pull/" + strconv.Itoa(pr),
		"pr " + strconv.Itoa(pr),
		"pr#" + strconv.Itoa(pr),
	} {
		if githubLiveContainsExactPRMarker(normalized, marker) {
			return true
		}
	}
	return false
}

func githubLiveContainsExactPRMarker(message string, marker string) bool {
	offset := 0
	for offset < len(message) {
		index := strings.Index(message[offset:], marker)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(marker)
		if githubLiveMarkerBoundary(message, index-1) && githubLiveMarkerBoundary(message, end) {
			return true
		}
		offset = index + 1
	}
	return false
}

func githubLiveMarkerBoundary(value string, index int) bool {
	if index < 0 || index >= len(value) {
		return true
	}
	ch := value[index]
	return !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_')
}

func githubLiveReviewCorrectionMarked(body string) bool {
	normalized := strings.ToLower(body)
	return strings.Contains(normalized, "relia:review-correction") ||
		strings.Contains(normalized, "relia:correction") ||
		strings.Contains(normalized, "relia-review-correction") ||
		strings.Contains(normalized, "relia-correction") ||
		strings.Contains(normalized, "relia correction") ||
		strings.Contains(normalized, "relia review correction")
}

func copyGitHubLiveString(target map[string]any, source map[string]any, targetKey string, sourcePaths ...string) {
	if value := githubString(source, sourcePaths...); value != "" {
		target[targetKey] = value
	}
}

func validGitHubPathSegment(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' ||
			r == '_' ||
			r == '.' {
			continue
		}
		return false
	}
	return true
}

func validEnvironmentVariableName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for index, r := range value {
		if (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			r == '_' ||
			(index > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func credentialRequiredError(message string, ref string) *Error {
	return &Error{Kind: ErrorCredential, Message: message, Ref: ref}
}

func rateLimitError(message string, ref string) *Error {
	return &Error{Kind: ErrorRateLimit, Message: message, Ref: ref}
}

func githubAPIError(message string, ref string) *Error {
	return &Error{Kind: ErrorGitHubAPI, Message: message, Ref: ref}
}
