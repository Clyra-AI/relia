package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var knownSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`(?i)\bgh[opusr]_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{20,}\b`),
}

type ErrorKind string

const (
	ErrorArtifactContract ErrorKind = "artifact_contract"
	ErrorCredential       ErrorKind = "credential_required"
	ErrorGitHubAPI        ErrorKind = "github_api"
	ErrorInternal         ErrorKind = "internal"
	ErrorProvenance       ErrorKind = "provenance_integrity"
	ErrorRateLimit        ErrorKind = "github_rate_limit"
	ErrorRedactionSafety  ErrorKind = "redaction_safety"
)

type Error struct {
	Kind    ErrorKind
	Message string
	Ref     string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Ref == "" {
		return e.Message
	}
	return e.Message + " (" + e.Ref + ")"
}

type Record struct {
	ObjectType      string         `json:"object_type"`
	SchemaVersion   string         `json:"schema_version"`
	ExperienceID    string         `json:"experience_id"`
	Repo            Repo           `json:"repo"`
	RecordedAt      string         `json:"recorded_at"`
	Attribution     Attribution    `json:"attribution"`
	Context         Context        `json:"context"`
	Action          Action         `json:"action"`
	Outcome         Outcome        `json:"outcome"`
	Provenance      Provenance     `json:"provenance"`
	FlakeDiscount   float64        `json:"flake_discount"`
	OrgEligible     bool           `json:"org_eligible"`
	ShareScope      string         `json:"share_scope"`
	RedactionStatus string         `json:"redaction_status"`
	Metadata        map[string]any `json:"metadata"`
}

type RecordOptions struct {
	SchemaVersion     string
	AttributionPolicy AttributionPolicy
	SourceIndex       int
}

type Repo struct {
	Provider string `json:"provider"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
}

type Attribution struct {
	ActorKind  string  `json:"actor_kind"`
	Method     string  `json:"method"`
	Confidence float64 `json:"confidence"`
}

type AttributionPolicy struct {
	PRLabels          []string
	CoauthorTrailers  []string
	AgentAuthorLogins []string
	Uncertain         string
}

type Context struct {
	Paths           []string `json:"paths"`
	DiffFingerprint string   `json:"diff_fingerprint"`
}

type Action struct {
	PR     int    `json:"pr"`
	Commit string `json:"commit"`
}

type Outcome struct {
	Kind          string    `json:"kind"`
	TerminalState string    `json:"terminal_state"`
	Signature     Signature `json:"signature"`
}

type Signature struct {
	SignatureID          string `json:"signature_id"`
	ExtractionConfidence string `json:"extraction_confidence"`
}

type Provenance struct {
	URLs []string `json:"urls"`
}

func ParseEvents(content []byte, ref string) ([]map[string]any, *Error) {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil, artifactContractError("ingest input is empty", ref)
	}
	var decoded any
	if err := DecodeJSONUseNumber(trimmed, &decoded); err == nil {
		return eventsFromJSON(decoded, ref)
	}
	var events []map[string]any
	for lineNumber, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event map[string]any
		if err := DecodeJSONUseNumber(line, &event); err != nil {
			return nil, artifactContractError(fmt.Sprintf("ingest JSONL line %d is not a JSON object", lineNumber+1), ref)
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, artifactContractError("ingest input contains no events", ref)
	}
	return events, nil
}

func DecodeJSONUseNumber(input string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func RedactForPersistence(event map[string]any, ref string) (any, *Error) {
	return redactValue(event, nil, ref)
}

func ValidateJSONRedactionSafe(content []byte, ref string) *Error {
	var decoded any
	if err := DecodeJSONUseNumber(string(content), &decoded); err != nil {
		return artifactContractError("ingest input must be valid JSON before redaction scan", ref)
	}
	_, commandErr := redactValue(decoded, nil, ref)
	return commandErr
}

func ValidGitHubProvenanceURLShape(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" &&
		strings.EqualFold(parsed.Host, "github.com") &&
		parsed.User == nil &&
		parsed.RawQuery == "" &&
		parsed.Fragment == "" &&
		strings.Trim(parsed.Path, "/") != ""
}

func ContainsStandardSecretTokenShape(value string) bool {
	for _, pattern := range knownSecretPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func PersistRecords(root string, records []Record) ([]string, *Error) {
	if len(records) == 0 {
		return []string{}, nil
	}
	grouped := map[string][]Record{}
	for _, record := range records {
		recordedAt, err := time.Parse(time.RFC3339, record.RecordedAt)
		if err != nil {
			return nil, artifactContractError("experience recorded_at must remain RFC3339 before persistence", record.ExperienceID)
		}
		shard := filepath.ToSlash(filepath.Join(".relia", "experiences", recordedAt.UTC().Format("2006-01")+".jsonl"))
		grouped[shard] = append(grouped[shard], record)
	}
	shards := make([]string, 0, len(grouped))
	for shard := range grouped {
		shards = append(shards, shard)
	}
	sort.Strings(shards)
	plans := make([]shardWritePlan, 0, len(shards))
	for _, shard := range shards {
		plan, commandErr := prepareShardWrite(filepath.Join(root, filepath.FromSlash(shard)), grouped[shard])
		if commandErr != nil {
			return nil, commandErr
		}
		plans = append(plans, plan)
	}
	for _, plan := range plans {
		if commandErr := writeShard(plan); commandErr != nil {
			return nil, commandErr
		}
	}
	return shards, nil
}

func GitHubProvenanceURLRepoMatchesRecord(value string, record Record) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	return len(parts) >= 2 &&
		strings.EqualFold(parts[0], record.Repo.Owner) &&
		strings.EqualFold(parts[1], record.Repo.Name)
}

func GitHubPullRequestURLPathNumber(value string) (int, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	if parsed.Scheme != "https" ||
		!strings.EqualFold(parsed.Host, "github.com") ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return 0, false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return 0, false
	}
	number, err := strconv.Atoi(parts[3])
	return number, err == nil && number > 0
}

func GitHubPullRequestURLNumber(value string) (int, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	if parsed.Scheme != "https" ||
		!strings.EqualFold(parsed.Host, "github.com") ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return 0, false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pull" {
		return 0, false
	}
	if parts[0] == "" || parts[1] == "" {
		return 0, false
	}
	number, err := strconv.Atoi(parts[3])
	return number, err == nil && number > 0
}

func GitHubPullRequestURLMatchesRecord(value string, record Record) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" ||
		!strings.EqualFold(parsed.Host, "github.com") ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pull" {
		return false
	}
	number, err := strconv.Atoi(parts[3])
	return err == nil &&
		number == record.Action.PR &&
		strings.EqualFold(parts[0], record.Repo.Owner) &&
		strings.EqualFold(parts[1], record.Repo.Name)
}

func GitHubPullRequestURLForRecord(record Record) string {
	owner := strings.Trim(strings.TrimSpace(record.Repo.Owner), "/")
	name := strings.Trim(strings.TrimSpace(record.Repo.Name), "/")
	if owner == "" || name == "" || record.Action.PR < 1 {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, name, record.Action.PR)
}

func PrimaryProvenanceURL(record Record) string {
	for _, value := range record.Provenance.URLs {
		if GitHubPullRequestURLMatchesRecord(value, record) {
			return value
		}
	}
	if derived := GitHubPullRequestURLForRecord(record); derived != "" {
		return derived
	}
	if len(record.Provenance.URLs) > 0 {
		return record.Provenance.URLs[0]
	}
	return ""
}

type shardWritePlan struct {
	Path    string
	Content []byte
}

func prepareShardWrite(path string, records []Record) (shardWritePlan, *Error) {
	order := []string{}
	byID := map[string]json.RawMessage{}
	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return shardWritePlan{}, internalError("could not read existing experience shard", err)
	}
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var existing map[string]any
		if err := json.Unmarshal([]byte(line), &existing); err != nil {
			return shardWritePlan{}, provenanceIntegrityError(fmt.Sprintf("existing experience shard line %d is not valid JSON", lineNumber+1), filepath.ToSlash(path))
		}
		experienceID := stringFromAny(existing["experience_id"])
		if experienceID == "" {
			return shardWritePlan{}, provenanceIntegrityError(fmt.Sprintf("existing experience shard line %d missing experience_id", lineNumber+1), filepath.ToSlash(path))
		}
		if _, ok := byID[experienceID]; !ok {
			order = append(order, experienceID)
		}
		byID[experienceID] = append(json.RawMessage(nil), []byte(line)...)
	}
	for _, record := range records {
		content, err := json.Marshal(record)
		if err != nil {
			return shardWritePlan{}, internalError("could not encode experience record", err)
		}
		if _, ok := byID[record.ExperienceID]; !ok {
			order = append(order, record.ExperienceID)
		}
		byID[record.ExperienceID] = content
	}
	var builder strings.Builder
	for _, experienceID := range order {
		builder.Write(byID[experienceID])
		builder.WriteByte('\n')
	}
	return shardWritePlan{Path: path, Content: []byte(builder.String())}, nil
}

func writeShard(plan shardWritePlan) *Error {
	if err := os.MkdirAll(filepath.Dir(plan.Path), 0o755); err != nil {
		return internalError("could not create experience shard directory", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(plan.Path), "."+filepath.Base(plan.Path)+".tmp-*")
	if err != nil {
		return internalError("could not create temporary experience shard", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := tempFile.Write(plan.Content); err != nil {
		_ = tempFile.Close()
		return internalError("could not write temporary experience shard", err)
	}
	if err := tempFile.Close(); err != nil {
		return internalError("could not close temporary experience shard", err)
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return internalError("could not set temporary experience shard permissions", err)
	}
	if err := os.Rename(tempPath, plan.Path); err != nil {
		return internalError("could not write experience shard", err)
	}
	cleanup = false
	return nil
}

func eventsFromJSON(value any, ref string) ([]map[string]any, *Error) {
	switch typed := value.(type) {
	case []any:
		return eventsFromArray(typed, ref)
	case map[string]any:
		if nested, ok := typed["events"]; ok {
			events, ok := nested.([]any)
			if !ok {
				return nil, artifactContractError("ingest events must be an array", ref)
			}
			return eventsFromArray(events, ref)
		}
		return []map[string]any{typed}, nil
	default:
		return nil, artifactContractError("ingest input must be a JSON object, array, or JSONL stream", ref)
	}
}

func eventsFromArray(values []any, ref string) ([]map[string]any, *Error) {
	if len(values) == 0 {
		return nil, artifactContractError("ingest input contains no events", ref)
	}
	events := make([]map[string]any, 0, len(values))
	for index, value := range values {
		event, ok := value.(map[string]any)
		if !ok {
			return nil, artifactContractError(fmt.Sprintf("ingest event %d must be a JSON object", index+1), ref)
		}
		events = append(events, event)
	}
	return events, nil
}

func redactValue(value any, fieldPath []string, ref string) (any, *Error) {
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, child := range typed {
			childPath := append(append([]string{}, fieldPath...), key)
			if commandErr := validateRedactedMapKey(key, fieldPath, ref); commandErr != nil {
				return nil, commandErr
			}
			if isSecretField(key) {
				redacted[key] = "[REDACTED:secret]"
				continue
			}
			next, commandErr := redactValue(child, childPath, ref)
			if commandErr != nil {
				return nil, commandErr
			}
			redacted[key] = next
		}
		return redacted, nil
	case []any:
		redacted := make([]any, 0, len(typed))
		for index, child := range typed {
			childPath := append(append([]string{}, fieldPath...), strconv.Itoa(index))
			next, commandErr := redactValue(child, childPath, ref)
			if commandErr != nil {
				return nil, commandErr
			}
			redacted = append(redacted, next)
		}
		return redacted, nil
	case []string:
		redacted := make([]string, 0, len(typed))
		for index, child := range typed {
			childPath := append(append([]string{}, fieldPath...), strconv.Itoa(index))
			next, commandErr := redactStringValue(child, childPath, ref)
			if commandErr != nil {
				return nil, commandErr
			}
			redacted = append(redacted, next)
		}
		return redacted, nil
	case string:
		return redactStringValue(typed, fieldPath, ref)
	default:
		return value, nil
	}
}

func validateRedactedMapKey(key string, fieldPath []string, ref string) *Error {
	keyPath := append(append([]string{}, fieldPath...), "<key>")
	redacted, commandErr := redactStringValue(key, keyPath, ref)
	if commandErr != nil {
		return commandErr
	}
	if redacted != key {
		pathRef := strings.Join(fieldPath, ".")
		if pathRef == "" {
			pathRef = "<root>"
		}
		return redactionSafetyError(fmt.Sprintf("secret-shaped object key at %s", pathRef), ref)
	}
	return nil
}

func redactStringValue(value string, fieldPath []string, ref string) (string, *Error) {
	redacted := value
	for _, pattern := range knownSecretPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED:token]")
	}
	if token := unsafeEntropyToken(redacted, fieldPath); token != "" {
		pathRef := strings.Join(fieldPath, ".")
		if pathRef == "" {
			pathRef = "<root>"
		}
		return "", redactionSafetyError(fmt.Sprintf("unrecognized high-entropy value at %s", pathRef), ref)
	}
	return redacted, nil
}

func isSecretField(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	switch normalized {
	case "token", "tokens", "secret", "secrets", "password", "passwords", "credential", "credentials", "api_key", "api_keys", "access_token", "access_tokens", "refresh_token", "refresh_tokens", "private_key", "private_keys", "client_secret", "client_secrets":
		return true
	}
	return strings.HasSuffix(normalized, "_token") ||
		strings.HasSuffix(normalized, "_tokens") ||
		strings.HasSuffix(normalized, "_secret") ||
		strings.HasSuffix(normalized, "_secrets") ||
		strings.HasSuffix(normalized, "_password") ||
		strings.HasSuffix(normalized, "_passwords") ||
		strings.Contains(normalized, "credential")
}

func unsafeEntropyToken(value string, fieldPath []string) string {
	if isGitHubProvenanceURLField(fieldPath) {
		if token := unsafeGitHubURLPathEntropyToken(value); token != "" {
			return token
		}
		if ValidGitHubProvenanceURLShape(value) {
			return ""
		}
	}
	if entropySafeFieldValue(fieldPath, value) {
		return ""
	}
	return unsafeEntropyTokenInString(value)
}

func unsafeEntropyTokenInString(value string) string {
	return unsafeEntropyTokenInStringWithSlashPolicy(value, true)
}

func unsafeEntropyTokenInPath(value string) string {
	return unsafeEntropyTokenInStringWithSlashPolicy(value, false)
}

func unsafeEntropyTokenInStringWithSlashPolicy(value string, allowSlash bool) string {
	candidates := strings.FieldsFunc(value, func(r rune) bool {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '+' || r == '_' || r == '-' || r == '=' {
			return false
		}
		if allowSlash && r == '/' {
			return false
		}
		return true
	})
	for _, candidate := range candidates {
		candidate = strings.Trim(candidate, "-_=+/")
		if len(candidate) < 32 {
			continue
		}
		if !hasMixedSecretAlphabet(candidate) {
			continue
		}
		if shannonEntropy(candidate) > 4.2 {
			return candidate
		}
	}
	return ""
}

func entropySafeFieldValue(fieldPath []string, value string) bool {
	for _, part := range fieldPath {
		normalized := strings.ToLower(part)
		switch normalized {
		case "commit", "commits":
			return validGitCommitHash(value)
		case "signature_id":
			return validSignatureIDValue(value)
		case "node_id":
			return true
		case "diff_fingerprint", "message_fingerprint", "digest", "checksum":
			return validHashLikeValue(value)
		}
		if strings.Contains(normalized, "fingerprint") {
			return validHashLikeValue(value)
		}
	}
	return false
}

func isGitHubProvenanceURLField(fieldPath []string) bool {
	for index, part := range fieldPath {
		normalized := strings.ToLower(part)
		switch normalized {
		case "pr_url", "check_run_url", "revert_url", "review_url", "provenance_urls":
			return true
		case "urls":
			if index > 0 && strings.ToLower(fieldPath[index-1]) == "provenance" {
				return true
			}
		}
	}
	return false
}

func unsafeGitHubURLPathEntropyToken(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") {
		return ""
	}
	for _, path := range []string{parsed.EscapedPath(), parsed.Path} {
		trimmed := strings.Trim(path, "/")
		if token := unsafeEntropyTokenInPath(trimmed); token != "" {
			return token
		}
		if token := unsafeSlashBearingEntropyTokenInPath(trimmed); token != "" {
			return token
		}
	}
	if unescaped, err := url.PathUnescape(parsed.EscapedPath()); err == nil {
		trimmed := strings.Trim(unescaped, "/")
		if token := unsafeEntropyTokenInPath(trimmed); token != "" {
			return token
		}
		if token := unsafeSlashBearingEntropyTokenInPath(trimmed); token != "" {
			return token
		}
	}
	return ""
}

func unsafeSlashBearingEntropyTokenInPath(path string) string {
	rawSegments := strings.Split(strings.Trim(path, "/"), "/")
	segments := unsafeGitHubPathTokenSegments(path)
	if len(segments) == 0 {
		return ""
	}
	for start := 0; start < len(segments); start++ {
		candidateSegments := make([]string, 0, len(segments)-start)
		for end := start; end < len(segments); end++ {
			segment := strings.Trim(segments[end], "-_=+")
			if segment == "" {
				if githubOwnerRepoRouteBoundary(rawSegments, end) &&
					(!suspiciousGitHubOwnerRepoTokenPrefix(candidateSegments) ||
						githubOwnerRepoRouteBoundaryHasSafeTypedPayload(rawSegments, end) ||
						githubOwnerRepoRouteBoundaryHasSafeUntypedPayload(rawSegments, end)) {
					candidateSegments = candidateSegments[:0]
				}
				continue
			}
			if !entropyPathCandidateFragment(segment) {
				break
			}
			candidateSegments = append(candidateSegments, segment)
			candidate := strings.Join(candidateSegments, "/")
			candidateWithoutSlash := strings.ReplaceAll(candidate, "/", "")
			if len(candidateSegments) < 2 {
				continue
			}
			if len(candidateWithoutSlash) < 32 {
				continue
			}
			if !hasMixedSecretAlphabet(candidateWithoutSlash) {
				continue
			}
			if shannonEntropy(candidate) > 4.2 {
				return candidate
			}
		}
	}
	return ""
}

func suspiciousGitHubOwnerRepoTokenPrefix(segments []string) bool {
	if len(segments) < 2 {
		return false
	}
	candidate := strings.Join(segments, "/")
	candidateWithoutSlash := strings.ReplaceAll(candidate, "/", "")
	if len(candidateWithoutSlash) < 16 {
		return false
	}
	if !hasMixedSecretAlphabet(candidateWithoutSlash) {
		return false
	}
	return shannonEntropy(candidate) > 3.2
}

func githubOwnerRepoRouteBoundary(rawSegments []string, index int) bool {
	if index != 2 || len(rawSegments) <= index {
		return false
	}
	route := strings.ToLower(strings.Trim(rawSegments[index], "-_=+"))
	if !safeGitHubRouteSegment(route) {
		return false
	}
	switch route {
	case "commit", "commits":
		return len(rawSegments) > index+1 && validGitCommitHash(rawSegments[index+1])
	case "pull", "pulls", "issues":
		return len(rawSegments) > index+1 && isDecimalString(rawSegments[index+1])
	case "runs":
		return len(rawSegments) > index+1 && isDecimalString(rawSegments[index+1])
	case "actions":
		return len(rawSegments) > index+1 && strings.EqualFold(rawSegments[index+1], "runs")
	case "checks", "suites", "workflow-runs", "tree", "blob", "compare":
		return len(rawSegments) > index+1
	default:
		return false
	}
}

func githubOwnerRepoRouteBoundaryHasSafeTypedPayload(rawSegments []string, index int) bool {
	if index != 2 || len(rawSegments) <= index {
		return false
	}
	route := strings.ToLower(strings.Trim(rawSegments[index], "-_=+"))
	switch route {
	case "commit", "commits":
		return len(rawSegments) > index+1 && validGitCommitHash(rawSegments[index+1])
	case "pull", "pulls", "issues":
		return len(rawSegments) > index+1 && isDecimalString(rawSegments[index+1])
	case "runs":
		return len(rawSegments) > index+1 && isDecimalString(rawSegments[index+1])
	case "actions":
		return len(rawSegments) > index+2 &&
			strings.EqualFold(strings.Trim(rawSegments[index+1], "-_=+"), "runs") &&
			isDecimalString(strings.Trim(rawSegments[index+2], "-_=+"))
	default:
		return false
	}
}

func githubOwnerRepoRouteBoundaryHasSafeUntypedPayload(rawSegments []string, index int) bool {
	if index != 2 || len(rawSegments) <= index {
		return false
	}
	route := strings.ToLower(strings.Trim(rawSegments[index], "-_=+"))
	switch route {
	case "tree", "blob":
		return githubUntypedRoutePayloadSafe(rawSegments[index+1:])
	default:
		return false
	}
}

func githubUntypedRoutePayloadSafe(rawSegments []string) bool {
	if len(rawSegments) == 0 {
		return false
	}
	for _, rawSegment := range rawSegments {
		segment := strings.Trim(rawSegment, "-_=+")
		if segment == "" {
			return false
		}
		if unsafeEntropyTokenInPath(segment) != "" {
			return false
		}
	}
	return unsafeSlashBearingEntropyTokenInPath(strings.Join(rawSegments, "/")) == ""
}

func unsafeGitHubPathTokenSegments(path string) []string {
	rawSegments := strings.Split(strings.Trim(path, "/"), "/")
	segments := make([]string, 0, len(rawSegments))
	for index, rawSegment := range rawSegments {
		segment := strings.Trim(rawSegment, "-_=+")
		if segment == "" {
			segments = append(segments, "")
			continue
		}
		if structuralGitHubRouteSegment(rawSegments, index) {
			segments = append(segments, "")
			continue
		}
		segments = append(segments, segment)
	}
	return segments
}

func structuralGitHubRouteSegment(rawSegments []string, index int) bool {
	if githubOwnerRepoRouteBoundary(rawSegments, index) {
		return true
	}
	if index == 3 &&
		len(rawSegments) > 3 &&
		strings.EqualFold(strings.Trim(rawSegments[2], "-_=+"), "actions") &&
		strings.EqualFold(strings.Trim(rawSegments[3], "-_=+"), "runs") {
		return true
	}
	return false
}

func safeGitHubRouteSegment(segment string) bool {
	normalized := strings.ToLower(segment)
	switch normalized {
	case "actions", "runs", "pull", "pulls", "issues", "commit", "commits", "tree", "blob", "compare", "checks", "suites", "workflow-runs":
		return true
	}
	return false
}

func isDecimalString(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func entropyPathCandidateFragment(segment string) bool {
	if segment == "" {
		return false
	}
	for _, r := range segment {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '+', r == '_', r == '-', r == '=':
		default:
			return false
		}
	}
	return true
}

func validGitCommitHash(value string) bool {
	return isHexString(strings.TrimSpace(value), 6, 64)
}

func validSignatureIDValue(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "sig_") {
		return isHexString(strings.TrimPrefix(value, "sig_"), 6, 64)
	}
	return validHashLikeValue(value)
}

func validHashLikeValue(value string) bool {
	value = strings.TrimSpace(value)
	if isHexString(value, 6, 128) {
		return true
	}
	for prefix, length := range map[string]int{
		"sha1:":   40,
		"sha256:": 64,
		"sha512:": 128,
	} {
		if strings.HasPrefix(strings.ToLower(value), prefix) {
			return isHexString(value[len(prefix):], length, length)
		}
	}
	return false
}

func isHexString(value string, minLength int, maxLength int) bool {
	if len(value) < minLength || len(value) > maxLength {
		return false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func hasMixedSecretAlphabet(value string) bool {
	hasLower := false
	hasUpper := false
	hasDigit := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	categories := 0
	for _, present := range []bool{hasLower, hasUpper, hasDigit} {
		if present {
			categories++
		}
	}
	return categories >= 2
}

func shannonEntropy(value string) float64 {
	if value == "" {
		return 0
	}
	counts := map[rune]int{}
	for _, r := range value {
		counts[r]++
	}
	length := float64(len([]rune(value)))
	entropy := 0.0
	for _, count := range counts {
		probability := float64(count) / length
		entropy -= probability * math.Log2(probability)
	}
	return entropy
}

func artifactContractError(message string, ref string) *Error {
	return &Error{Kind: ErrorArtifactContract, Message: message, Ref: ref}
}

func internalError(message string, err error) *Error {
	if err != nil {
		message += ": " + err.Error()
	}
	return &Error{Kind: ErrorInternal, Message: message}
}

func provenanceIntegrityError(message string, ref string) *Error {
	return &Error{Kind: ErrorProvenance, Message: message, Ref: ref}
}

func redactionSafetyError(message string, ref string) *Error {
	return &Error{Kind: ErrorRedactionSafety, Message: message, Ref: ref}
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}
