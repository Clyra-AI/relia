package ingest

import (
	"fmt"
	"sort"
	"strings"
)

func ParseGitHubOutcomeEvents(content []byte, ref string) ([]map[string]any, *Error) {
	var decoded any
	if err := DecodeJSONUseNumber(string(content), &decoded); err != nil {
		return nil, artifactContractError("github outcome input must be a JSON object", ref)
	}
	envelope, ok := decoded.(map[string]any)
	if !ok {
		return nil, artifactContractError("github outcome input must be a JSON object", ref)
	}
	pulls := githubOutcomeObjects(envelope, "pull_requests", "prs")
	if len(pulls) == 0 {
		return nil, artifactContractError("github outcome input must include pull_requests", ref)
	}
	defaultRepo := Repo{Provider: "github"}
	if githubEnvelopeHasRepo(envelope) {
		var commandErr *Error
		defaultRepo, commandErr = NormalizeRepo(envelope, ref)
		if commandErr != nil {
			return nil, commandErr
		}
	}

	var events []map[string]any
	for index, pull := range pulls {
		base, commandErr := githubOutcomeBase(defaultRepo, pull, ref, index)
		if commandErr != nil {
			return nil, commandErr
		}
		checkRuns := githubOutcomeObjects(pull, "check_runs", "checks")
		reverts := githubOutcomeObjects(pull, "reverts")
		reviewCorrections := githubMarkedReviewCorrections(pull)
		failingChecks := 0
		for _, checkRun := range checkRuns {
			if !githubCheckRunFailed(checkRun) {
				continue
			}
			failingChecks++
			events = append(events, githubOutcomeEvent(base, checkRun, githubOutcomeEventOptions{
				Kind:           "ci_failure",
				RecordedAt:     githubString(checkRun, "completed_at", "updated_at", "created_at"),
				Commit:         githubString(checkRun, "head_sha", "commit"),
				Paths:          githubPaths(checkRun, base.Paths),
				CheckName:      githubString(checkRun, "name", "check_name"),
				SignatureClass: "test_failure",
				SignatureKey:   githubSignatureKey(checkRun, base.Paths),
				Message:        githubString(checkRun, "message", "summary", "output.summary", "conclusion"),
				ProvenanceURL:  githubString(checkRun, "html_url", "details_url", "url", "check_run_url"),
			}))
		}
		for _, revert := range reverts {
			events = append(events, githubOutcomeEvent(base, revert, githubOutcomeEventOptions{
				Kind:           "revert",
				RecordedAt:     githubString(revert, "created_at", "merged_at", "committed_at", "recorded_at"),
				SourceCommit:   githubString(revert, "commit_sha", "sha", "commit"),
				Paths:          githubPaths(revert, base.Paths),
				CheckName:      "revert",
				SignatureClass: "revert",
				SignatureKey:   githubSignatureKey(revert, base.Paths),
				Message:        githubString(revert, "message", "title"),
				ProvenanceURL:  githubString(revert, "html_url", "url", "commit_url", "revert_url"),
			}))
		}
		for _, correction := range reviewCorrections {
			events = append(events, githubOutcomeEvent(base, correction, githubOutcomeEventOptions{
				Kind:           "review_correction",
				RecordedAt:     githubString(correction, "resolved_at", "updated_at", "created_at", "recorded_at"),
				Commit:         githubString(correction, "commit_id", "commit", "head_sha"),
				Paths:          githubPaths(correction, base.Paths),
				CheckName:      "review_correction",
				SignatureClass: "review_correction",
				SignatureKey:   githubSignatureKey(correction, base.Paths),
				Message:        githubString(correction, "message", "body", "title"),
				ProvenanceURL:  githubString(correction, "html_url", "url", "review_url"),
			}))
		}
		if githubString(pull, "merged_at") != "" && failingChecks == 0 && len(reverts) == 0 && len(reviewCorrections) == 0 {
			events = append(events, githubOutcomeEvent(base, pull, githubOutcomeEventOptions{
				Kind:           "merged_clean",
				RecordedAt:     githubString(pull, "merged_at", "updated_at", "created_at"),
				Commit:         base.Commit,
				Paths:          base.Paths,
				CheckName:      "merge",
				SignatureClass: "unknown",
				SignatureKey:   firstString(firstString(base.Paths...), base.Commit),
				ProvenanceURL:  base.PRURL,
			}))
		}
	}
	if len(events) == 0 {
		return nil, artifactContractError("github outcome input produced no supported outcomes", ref)
	}
	sort.SliceStable(events, func(left, right int) bool {
		leftPR, _ := intFromAny(events[left]["pr"])
		rightPR, _ := intFromAny(events[right]["pr"])
		if leftPR != rightPR {
			return leftPR < rightPR
		}
		leftRecorded := stringFromAny(events[left]["recorded_at"])
		rightRecorded := stringFromAny(events[right]["recorded_at"])
		if leftRecorded != rightRecorded {
			return leftRecorded < rightRecorded
		}
		return stringFromAny(events[left]["outcome_kind"]) < stringFromAny(events[right]["outcome_kind"])
	})
	return events, nil
}

func githubEnvelopeHasRepo(envelope map[string]any) bool {
	for _, path := range []string{"repo", "repo_owner", "repo_name"} {
		if _, ok := nestedField(envelope, path); ok {
			return true
		}
	}
	return false
}

type githubOutcomeBaseFields struct {
	Repo      Repo
	PR        int
	Commit    string
	PRURL     string
	Paths     []string
	Labels    []string
	Coauthors []string
	Actor     string
}

type githubOutcomeEventOptions struct {
	Kind           string
	RecordedAt     string
	Commit         string
	Paths          []string
	CheckName      string
	SignatureClass string
	SignatureKey   string
	Message        string
	ProvenanceURL  string
	SourceCommit   string
}

func githubOutcomeBase(defaultRepo Repo, pull map[string]any, ref string, index int) (githubOutcomeBaseFields, *Error) {
	repo := defaultRepo
	if githubEnvelopeHasRepo(pull) {
		candidate, commandErr := NormalizeRepo(pull, ref)
		if commandErr != nil {
			return githubOutcomeBaseFields{}, commandErr
		}
		repo = candidate
	}
	pr, ok := intFromAny(firstAny(pull, "number", "pr", "action.pr"))
	if !ok || pr < 1 {
		return githubOutcomeBaseFields{}, provenanceIntegrityError(fmt.Sprintf("github pull request %d number must be a positive integer", index+1), ref)
	}
	commit := githubString(pull, "head_sha", "merge_commit_sha", "commit", "head.sha")
	if commit == "" {
		return githubOutcomeBaseFields{}, artifactContractError(fmt.Sprintf("github pull request %d must include head_sha or commit", pr), ref)
	}
	prURL := githubString(pull, "html_url", "url", "pr_url")
	if prURL == "" {
		prURL = fmt.Sprintf("https://github.com/%s/%s/pull/%d", repo.Owner, repo.Name, pr)
	}
	if !ValidGitHubProvenanceURLShape(prURL) {
		return githubOutcomeBaseFields{}, provenanceIntegrityError(fmt.Sprintf("github pull request %d URL must be a canonical https://github.com/ URL", pr), ref)
	}
	paths := githubPaths(pull, nil)
	if len(paths) == 0 {
		return githubOutcomeBaseFields{}, artifactContractError(fmt.Sprintf("github pull request %d must include changed files", pr), ref)
	}
	return githubOutcomeBaseFields{
		Repo:      repo,
		PR:        pr,
		Commit:    commit,
		PRURL:     prURL,
		Paths:     paths,
		Labels:    githubLabels(pull),
		Coauthors: stringListField(pull, "coauthors", "coauthor_trailers"),
		Actor:     githubString(pull, "author.login", "user.login", "actor.login", "author", "user", "actor"),
	}, nil
}

func githubOutcomeEvent(base githubOutcomeBaseFields, source map[string]any, options githubOutcomeEventOptions) map[string]any {
	commit := firstString(options.Commit, base.Commit)
	paths := options.Paths
	if len(paths) == 0 {
		paths = base.Paths
	}
	provenanceURLs := []string{base.PRURL}
	if provenanceURL := strings.TrimSpace(options.ProvenanceURL); provenanceURL != "" && ValidGitHubProvenanceURLShape(provenanceURL) {
		provenanceURLs = uniqueStrings(append(provenanceURLs, provenanceURL))
	}
	event := map[string]any{
		"repo": map[string]any{
			"provider": base.Repo.Provider,
			"owner":    base.Repo.Owner,
			"name":     base.Repo.Name,
		},
		"recorded_at":           options.RecordedAt,
		"pr":                    base.PR,
		"commit":                commit,
		"paths":                 paths,
		"labels":                base.Labels,
		"coauthors":             base.Coauthors,
		"actor":                 map[string]any{"login": base.Actor},
		"outcome_kind":          options.Kind,
		"signature_class":       options.SignatureClass,
		"check_name":            firstString(options.CheckName, options.Kind),
		"signature_key":         firstString(options.SignatureKey, firstString(paths...)),
		"extraction_confidence": "structured",
		"provenance_urls":       provenanceURLs,
		"metadata": map[string]any{
			"source_format": "github_structured_export",
		},
	}
	if options.SourceCommit != "" {
		event["metadata"].(map[string]any)["github_source_commit"] = options.SourceCommit
	}
	if options.Message != "" {
		event["message"] = options.Message
	}
	if event["recorded_at"] == "" {
		event["recorded_at"] = githubString(source, "updated_at", "created_at")
	}
	return event
}

func githubOutcomeObjects(event map[string]any, paths ...string) []map[string]any {
	for _, path := range paths {
		value, ok := nestedField(event, path)
		if !ok {
			continue
		}
		items, ok := value.([]any)
		if !ok {
			continue
		}
		var result []map[string]any
		for _, item := range items {
			if mapping, ok := item.(map[string]any); ok {
				result = append(result, mapping)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return nil
}

func githubMarkedReviewCorrections(pull map[string]any) []map[string]any {
	var result []map[string]any
	for _, correction := range githubOutcomeObjects(pull, "review_corrections") {
		if githubBool(correction, "marked", "correction", "requires_correction", "is_correction") {
			result = append(result, correction)
		}
	}
	result = append(result, githubOutcomeObjects(pull, "marked_review_corrections")...)
	return result
}

func githubCheckRunFailed(checkRun map[string]any) bool {
	switch strings.ToLower(githubString(checkRun, "conclusion", "status")) {
	case "failure", "failed", "timed_out", "action_required", "cancelled":
		return true
	default:
		return false
	}
}

func githubPaths(event map[string]any, fallback []string) []string {
	paths := stringListField(event, "paths", "changed_files", "files")
	for _, file := range githubOutcomeObjects(event, "files") {
		if path := githubString(file, "filename", "path"); path != "" {
			paths = append(paths, path)
		}
	}
	paths = uniqueStrings(paths)
	if len(paths) > 0 {
		return paths
	}
	return append([]string(nil), fallback...)
}

func githubLabels(event map[string]any) []string {
	labels := stringListField(event, "labels", "label_names")
	for _, label := range githubOutcomeObjects(event, "labels") {
		if name := githubString(label, "name"); name != "" {
			labels = append(labels, name)
		}
	}
	return uniqueStrings(labels)
}

func githubSignatureKey(event map[string]any, fallback []string) string {
	return firstString(
		githubString(event, "signature_key", "path", "filename"),
		firstString(githubPaths(event, fallback)...),
	)
}

func githubString(event map[string]any, paths ...string) string {
	for _, path := range paths {
		if value, ok := nestedField(event, path); ok {
			if converted := stringFromAny(value); converted != "" {
				return converted
			}
		}
	}
	return ""
}

func githubBool(event map[string]any, paths ...string) bool {
	for _, path := range paths {
		value, ok := nestedField(event, path)
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			if typed {
				return true
			}
		case string:
			normalized := strings.ToLower(strings.TrimSpace(typed))
			if normalized == "true" || normalized == "yes" || normalized == "required" || normalized == "correction" {
				return true
			}
		}
	}
	return false
}

func firstAny(event map[string]any, paths ...string) any {
	for _, path := range paths {
		if value, ok := nestedField(event, path); ok {
			return value
		}
	}
	return nil
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
