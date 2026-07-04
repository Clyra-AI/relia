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
	if commandErr := ValidateEventMemorySource(envelope, ref); commandErr != nil {
		return nil, commandErr
	}
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
			event, commandErr := githubOutcomeEvent(base, checkRun, githubOutcomeEventOptions{
				Kind:           "ci_failure",
				RecordedAt:     githubString(checkRun, "completed_at", "updated_at", "created_at"),
				Commit:         githubString(checkRun, "head_sha", "commit"),
				Paths:          githubPaths(checkRun, base.Paths),
				CheckName:      githubCheckName(checkRun, "name", "ci_failure"),
				SignatureClass: githubSignatureClass(checkRun, "test_failure"),
				SignatureKey:   githubSignatureKey(checkRun, base.Paths),
				Message:        githubString(checkRun, "message", "summary", "output.summary", "conclusion"),
				ProvenanceURL:  githubString(checkRun, "html_url", "details_url", "url", "check_run_url"),
			}, ref)
			if commandErr != nil {
				return nil, commandErr
			}
			events = append(events, event)
		}
		for _, revert := range reverts {
			event, commandErr := githubOutcomeEvent(base, revert, githubOutcomeEventOptions{
				Kind:           "revert",
				RecordedAt:     githubString(revert, "created_at", "merged_at", "committed_at", "recorded_at"),
				SourceCommit:   githubString(revert, "commit_sha", "sha", "commit"),
				Paths:          githubPaths(revert, base.Paths),
				CheckName:      githubCheckName(revert, "revert"),
				SignatureClass: githubSignatureClass(revert, "revert"),
				SignatureKey:   githubSignatureKey(revert, base.Paths),
				Message:        githubString(revert, "message", "title"),
				ProvenanceURL:  githubString(revert, "html_url", "url", "commit_url", "revert_url"),
			}, ref)
			if commandErr != nil {
				return nil, commandErr
			}
			events = append(events, event)
		}
		for _, correction := range reviewCorrections {
			event, commandErr := githubOutcomeEvent(base, correction, githubOutcomeEventOptions{
				Kind:           "review_correction",
				RecordedAt:     githubString(correction, "resolved_at", "updated_at", "created_at", "recorded_at"),
				Commit:         githubString(correction, "commit_id", "commit", "head_sha"),
				Paths:          githubPaths(correction, base.Paths),
				CheckName:      githubCheckName(correction, "review_correction"),
				SignatureClass: githubSignatureClass(correction, "review_correction"),
				SignatureKey:   githubSignatureKey(correction, base.Paths),
				Message:        githubString(correction, "message", "body", "title"),
				ProvenanceURL:  githubString(correction, "html_url", "url", "review_url"),
			}, ref)
			if commandErr != nil {
				return nil, commandErr
			}
			events = append(events, event)
		}
		if githubString(pull, "merged_at") != "" && failingChecks == 0 && len(reverts) == 0 && len(reviewCorrections) == 0 {
			event, commandErr := githubOutcomeEvent(base, pull, githubOutcomeEventOptions{
				Kind:           "merged_clean",
				RecordedAt:     githubString(pull, "merged_at", "updated_at", "created_at"),
				Commit:         base.Commit,
				Paths:          base.Paths,
				CheckName:      firstString(githubCheckName(pull), "merge"),
				SignatureClass: githubSignatureClass(pull, "unknown"),
				SignatureKey:   githubSignatureKey(pull, base.Paths),
				ProvenanceURL:  base.PRURL,
			}, ref)
			if commandErr != nil {
				return nil, commandErr
			}
			events = append(events, event)
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
	if value, ok := nestedField(envelope, "repo"); ok {
		switch typed := value.(type) {
		case map[string]any:
			return strings.TrimSpace(stringFromAny(typed["owner"])) != "" &&
				strings.TrimSpace(stringFromAny(typed["name"])) != ""
		case string:
			owner, name, ok := strings.Cut(strings.TrimSpace(typed), "/")
			return ok && strings.TrimSpace(owner) != "" && strings.TrimSpace(name) != ""
		}
	}
	return githubString(envelope, "repo_owner") != "" && githubString(envelope, "repo_name") != ""
}

type githubOutcomeBaseFields struct {
	Repo                  Repo
	PR                    int
	Commit                string
	PRURL                 string
	Paths                 []string
	Labels                []string
	Coauthors             []string
	Actor                 string
	ActorKind             string
	AttributionMethod     string
	AttributionConfidence any
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
	if commandErr := ValidateEventMemorySource(pull, ref); commandErr != nil {
		return githubOutcomeBaseFields{}, commandErr
	}
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
	commit := githubString(pull, "head_sha", "head.sha", "commit", "merge_commit_sha")
	prURL := githubString(pull, "html_url", "pr_url")
	if prURL == "" {
		if candidate := githubString(pull, "url"); ValidGitHubProvenanceURLShape(candidate) {
			prURL = candidate
		}
	}
	if prURL == "" {
		prURL = fmt.Sprintf("https://github.com/%s/%s/pull/%d", repo.Owner, repo.Name, pr)
	}
	if !ValidGitHubProvenanceURLShape(prURL) {
		return githubOutcomeBaseFields{}, provenanceIntegrityError(fmt.Sprintf("github pull request %d URL must be a canonical https://github.com/ URL", pr), ref)
	}
	repo = githubRepoFromPRURL(repo, prURL)
	if !githubPullRequestURLMatchesBase(repo, pr, prURL) {
		return githubOutcomeBaseFields{}, provenanceIntegrityError(fmt.Sprintf("github pull request %d URL must match repo and pull number", pr), ref)
	}
	paths := githubPaths(pull, nil)
	return githubOutcomeBaseFields{
		Repo:                  repo,
		PR:                    pr,
		Commit:                commit,
		PRURL:                 prURL,
		Paths:                 paths,
		Labels:                githubLabels(pull),
		Coauthors:             stringListField(pull, "coauthors", "coauthor_trailers"),
		Actor:                 githubString(pull, "author.login", "user.login", "actor.login", "author", "user", "actor"),
		ActorKind:             githubString(pull, "actor_kind", "attribution.actor_kind"),
		AttributionMethod:     githubString(pull, "attribution_method", "attribution.method"),
		AttributionConfidence: githubFirstAny(pull, "attribution_confidence", "attribution.confidence"),
	}, nil
}

func githubRepoFromPRURL(repo Repo, prURL string) Repo {
	if repo.Owner != "" && repo.Name != "" {
		return repo
	}
	const prefix = "https://github.com/"
	if !strings.HasPrefix(prURL, prefix) {
		return repo
	}
	parts := strings.Split(strings.TrimPrefix(prURL, prefix), "/")
	if len(parts) >= 4 && parts[0] != "" && parts[1] != "" && parts[2] == "pull" {
		repo.Owner = parts[0]
		repo.Name = parts[1]
	}
	return repo
}

func githubPullRequestURLMatchesBase(repo Repo, pr int, prURL string) bool {
	urlRepo := githubRepoFromPRURL(Repo{}, prURL)
	urlPR, ok := GitHubPullRequestURLNumber(prURL)
	return ok &&
		urlPR == pr &&
		strings.EqualFold(urlRepo.Owner, repo.Owner) &&
		strings.EqualFold(urlRepo.Name, repo.Name)
}

func githubProvenanceURLMatchesBase(repo Repo, pr int, provenanceURL string) bool {
	const prefix = "https://github.com/"
	if !strings.HasPrefix(provenanceURL, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(provenanceURL, prefix), "/")
	if len(parts) < 2 ||
		!strings.EqualFold(parts[0], repo.Owner) ||
		!strings.EqualFold(parts[1], repo.Name) {
		return false
	}
	if urlPR, ok := GitHubPullRequestURLPathNumber(provenanceURL); ok && urlPR != pr {
		return false
	}
	return true
}

func githubOutcomeEvent(base githubOutcomeBaseFields, source map[string]any, options githubOutcomeEventOptions, ref string) (map[string]any, *Error) {
	if commandErr := ValidateEventMemorySource(source, ref); commandErr != nil {
		return nil, commandErr
	}
	commit := firstString(options.Commit, base.Commit)
	if commit == "" {
		return nil, artifactContractError(fmt.Sprintf("github pull request %d %s outcome must include head_sha or commit", base.PR, options.Kind), ref)
	}
	paths := options.Paths
	if len(paths) == 0 {
		paths = base.Paths
	}
	if len(paths) == 0 {
		return nil, artifactContractError(fmt.Sprintf("github pull request %d %s outcome must include changed files", base.PR, options.Kind), ref)
	}
	provenanceURLs := []string{base.PRURL}
	if provenanceURL := strings.TrimSpace(options.ProvenanceURL); provenanceURL != "" && ValidGitHubProvenanceURLShape(provenanceURL) {
		if !githubProvenanceURLMatchesBase(base.Repo, base.PR, provenanceURL) {
			return nil, provenanceIntegrityError(fmt.Sprintf("github pull request %d %s outcome provenance URL must match repo and pull number", base.PR, options.Kind), ref)
		}
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
	if signatureID := githubString(source, "signature_id", "outcome.signature.signature_id"); signatureID != "" {
		event["signature_id"] = signatureID
	}
	if extractionConfidence := githubString(source, "extraction_confidence", "outcome.signature.extraction_confidence"); extractionConfidence != "" {
		event["extraction_confidence"] = extractionConfidence
	}
	if flakeDiscount, ok := githubOptionalAny(source, "flake_discount", "outcome.flake_discount"); ok {
		event["flake_discount"] = flakeDiscount
	}
	if messageFingerprint := githubString(source, "message_fingerprint", "outcome.signature.message_fingerprint"); messageFingerprint != "" {
		event["message_fingerprint"] = messageFingerprint
	}
	if actorKind := firstString(githubString(source, "actor_kind", "attribution.actor_kind"), base.ActorKind); actorKind != "" {
		event["actor_kind"] = actorKind
	}
	if attributionMethod := firstString(githubString(source, "attribution_method", "attribution.method"), base.AttributionMethod); attributionMethod != "" {
		event["attribution_method"] = attributionMethod
	}
	if attributionConfidence := githubFirstAny(source, "attribution_confidence", "attribution.confidence"); attributionConfidence != nil {
		event["attribution_confidence"] = attributionConfidence
	} else if base.AttributionConfidence != nil {
		event["attribution_confidence"] = base.AttributionConfidence
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
	return event, nil
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
	case "failure", "failed", "timed_out", "action_required":
		return true
	default:
		return false
	}
}

func githubPaths(event map[string]any, fallback []string) []string {
	paths := stringListField(event, "paths", "changed_files", "files", "path", "filename")
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

func githubCheckName(event map[string]any, fallback ...string) string {
	return firstString(
		githubString(event, "check_name", "outcome.signature.check_name", "outcome.signature.check"),
		githubString(event, fallback...),
	)
}

func githubSignatureClass(event map[string]any, fallback string) string {
	return firstString(githubString(event, "signature_class", "outcome.signature.class"), fallback)
}

func githubSignatureKey(event map[string]any, fallback []string) string {
	return firstString(
		githubString(event, "signature_key", "outcome.signature.key", "path", "filename"),
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

func githubFirstAny(event map[string]any, paths ...string) any {
	for _, path := range paths {
		if value, ok := nestedField(event, path); ok {
			return value
		}
	}
	return nil
}

func githubOptionalAny(event map[string]any, paths ...string) (any, bool) {
	for _, path := range paths {
		if value, ok := nestedField(event, path); ok {
			if value == nil {
				continue
			}
			return value, true
		}
	}
	return nil, false
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
