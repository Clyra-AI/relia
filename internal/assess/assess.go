package assess

import (
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
	resultdoc "github.com/Clyra-AI/relia/internal/result"
	"github.com/Clyra-AI/relia/internal/yamlmini"
)

const objectType = "relia.risk_assessment"

type Options struct {
	SchemaVersion            string
	ProvenanceIntegrityError func(string, string) *resultdoc.CommandError
}

type RiskAssessment struct {
	ObjectType    string                `json:"object_type"`
	SchemaVersion string                `json:"schema_version"`
	AssessmentID  string                `json:"assessment_id"`
	RiskLevel     string                `json:"risk_level"`
	Matches       []RiskAssessmentMatch `json:"matches"`
	Citations     []string              `json:"citations"`
	Metadata      map[string]any        `json:"metadata"`
}

type RiskAssessmentMatch struct {
	RuleID     string  `json:"rule_id"`
	Confidence float64 `json:"confidence"`
}

type Rule struct {
	ID         string
	Kind       string
	Path       string
	Confidence float64
	ScopePaths []string
	Citations  []RuleCitation
}

type RuleCitation struct {
	URL     string
	PR      int
	Outcome string
}

func ServedRuleData(rules []Rule, options Options) ([]map[string]any, *resultdoc.CommandError) {
	data := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		servedCitationRefs := ServedRuleCitations(rule)
		servedCitations := RuleCitationURLs(servedCitationRefs)
		if len(servedCitations) == 0 {
			return nil, provenanceError(options, "served active memory rule must include citation URLs", rule.Path)
		}
		if commandErr := ValidateServedRuleCitations(rule, servedCitationRefs, options); commandErr != nil {
			return nil, commandErr
		}
		data = append(data, map[string]any{
			"rule_id":     rule.ID,
			"kind":        rule.Kind,
			"confidence":  rule.Confidence,
			"scope_paths": append([]string(nil), rule.ScopePaths...),
			"citations":   servedCitations,
			"path":        rule.Path,
		})
	}
	return data, nil
}

func BuildRiskAssessment(root string, inputRef string, content []byte, touchedPaths []string, rules []Rule, options Options) (RiskAssessment, *resultdoc.CommandError) {
	matches := []RiskAssessmentMatch{}
	citations := []string{}
	highestAvoidConfidence := -1.0
	hasPlaybookCoverage := false
	for _, rule := range rules {
		if !RuleMatchesTouchedPath(root, rule, touchedPaths) {
			continue
		}
		if strings.TrimSpace(rule.ID) == "" {
			return RiskAssessment{}, provenanceError(options, "matched active memory rule id must be non-empty for assessment", rule.Path)
		}
		servedCitationRefs := ServedRuleCitations(rule)
		servedCitations := RuleCitationURLs(servedCitationRefs)
		if len(servedCitations) == 0 {
			return RiskAssessment{}, provenanceError(options, "matched active memory rule must include citation URLs for assessment", rule.Path)
		}
		if math.IsNaN(rule.Confidence) || math.IsInf(rule.Confidence, 0) || rule.Confidence < 0 || rule.Confidence > 1 {
			return RiskAssessment{}, provenanceError(options, "matched active memory rule confidence must be between 0 and 1 for assessment", rule.Path)
		}
		if commandErr := ValidateServedRuleCitations(rule, servedCitationRefs, options); commandErr != nil {
			return RiskAssessment{}, commandErr
		}
		matches = append(matches, RiskAssessmentMatch{
			RuleID:     rule.ID,
			Confidence: rule.Confidence,
		})
		citations = append(citations, servedCitations...)
		if rule.Kind == "playbook" {
			hasPlaybookCoverage = true
		} else if rule.Confidence > highestAvoidConfidence {
			highestAvoidConfidence = rule.Confidence
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Confidence == matches[j].Confidence {
			return matches[i].RuleID < matches[j].RuleID
		}
		return matches[i].Confidence > matches[j].Confidence
	})
	citations = uniqueStrings(citations)
	if citations == nil {
		citations = []string{}
	}
	riskLevel := "no_coverage"
	if highestAvoidConfidence >= 0.75 {
		riskLevel = "match_high"
	} else if highestAvoidConfidence >= 0 {
		riskLevel = "match_medium"
	} else if hasPlaybookCoverage {
		riskLevel = "covered_clean"
	}
	metadata := map[string]any{
		"input_path":               inputRef,
		"diff_fingerprint":         sha256String(string(content)),
		"touched_paths":            touchedPaths,
		"repo_relative_paths_only": true,
		"redaction_status":         "customer_safe",
	}
	if highestAvoidConfidence >= 0 {
		metadata["max_avoid_confidence"] = highestAvoidConfidence
	}
	return RiskAssessment{
		ObjectType:    objectType,
		SchemaVersion: options.SchemaVersion,
		AssessmentID:  "assess_" + shortHash(inputRef+"|"+sha256String(string(content))+"|"+strings.Join(touchedPaths, "\x00")),
		RiskLevel:     riskLevel,
		Matches:       matches,
		Citations:     citations,
		Metadata:      metadata,
	}, nil
}

func HasPositivePlaybookEvidence(document yamlmini.Document) bool {
	for _, provenance := range document.ListMaps["provenance"] {
		outcome, ok := provenance["outcome"]
		if !ok {
			continue
		}
		if outcome.Value == "fix_held" || outcome.Value == "merged_clean" {
			return true
		}
	}
	return false
}

func RuleCitations(document yamlmini.Document) []RuleCitation {
	var citations []RuleCitation
	for _, provenance := range document.ListMaps["provenance"] {
		url, ok := provenance["url"]
		if !ok || strings.TrimSpace(url.Value) == "" {
			continue
		}
		prNumber := 0
		if pr, ok := provenance["pr"]; ok {
			prNumber, _ = strconv.Atoi(pr.Value)
		}
		outcome := ""
		if value, ok := provenance["outcome"]; ok {
			outcome = value.Value
		}
		citations = append(citations, RuleCitation{
			URL:     url.Value,
			PR:      prNumber,
			Outcome: outcome,
		})
	}
	return UniqueRuleCitations(citations)
}

func UniqueRuleCitations(citations []RuleCitation) []RuleCitation {
	seen := map[string]bool{}
	var unique []RuleCitation
	for _, citation := range citations {
		key := fmt.Sprintf("%s\x00%d\x00%s", citation.URL, citation.PR, citation.Outcome)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, citation)
	}
	return unique
}

func ValidateServedRuleCitations(rule Rule, servedCitationRefs []RuleCitation, options Options) *resultdoc.CommandError {
	for _, citation := range servedCitationRefs {
		prNumber, ok := ingestdoc.GitHubPullRequestURLNumber(citation.URL)
		if !ok {
			return provenanceError(options, "matched active memory rule citation URL must be an https://github.com/<owner>/<repo>/pull/<number> URL", rule.Path)
		}
		if citation.PR <= 0 || prNumber != citation.PR {
			return provenanceError(options, "matched active memory rule citation URL pull number must match provenance pr", rule.Path)
		}
	}
	return nil
}

func ServedRuleCitationURLs(rule Rule) []string {
	return RuleCitationURLs(ServedRuleCitations(rule))
}

func ServedRuleCitations(rule Rule) []RuleCitation {
	var refs []RuleCitation
	for _, citation := range rule.Citations {
		if rule.Kind == "playbook" && citation.Outcome != "fix_held" && citation.Outcome != "merged_clean" {
			continue
		}
		refs = append(refs, citation)
	}
	return UniqueRuleCitations(refs)
}

func RuleCitationURLs(refs []RuleCitation) []string {
	var citations []string
	for _, citation := range refs {
		citations = append(citations, citation.URL)
	}
	return uniqueStrings(citations)
}

func RuleMatchesTouchedPath(root string, rule Rule, touchedPaths []string) bool {
	for _, rawScopePath := range rule.ScopePaths {
		scopePath, directoryScope, ok := NormalizeScopePath(root, rawScopePath)
		if !ok {
			continue
		}
		for _, touchedPath := range touchedPaths {
			touchedPath = filepath.ToSlash(filepath.Clean(filepath.FromSlash(touchedPath)))
			if scopePatternMatches(scopePath, touchedPath) || scopePath == touchedPath || DirectoryScopeMatches(scopePath, touchedPath, directoryScope) {
				return true
			}
		}
	}
	return false
}

func NormalizeScopePath(root string, raw string) (string, bool, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false, false
	}
	slashPath := filepath.ToSlash(trimmed)
	directoryScope := strings.HasSuffix(slashPath, "/") && !hasGlobMagic(slashPath)
	clean, ok := cleanRepoPath(slashPath)
	if !ok {
		return "", false, false
	}
	scopePath := filepath.ToSlash(clean)
	if !directoryScope && !hasGlobMagic(scopePath) {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(scopePath)))
		if err == nil {
			directoryScope = info.IsDir()
		} else if HistoricalDirectoryScope(root, scopePath) {
			directoryScope = true
		}
	}
	return scopePath, directoryScope, true
}

func DirectoryScopeMatches(scopePath string, touchedPath string, directoryScope bool) bool {
	if !directoryScope {
		return false
	}
	return touchedPath == scopePath || strings.HasPrefix(touchedPath, scopePath+"/")
}

func HistoricalDirectoryScope(root string, scopePath string) bool {
	output, err := exec.Command("git", "-C", root, "log", "--all", "--name-only", "--format=", "--", scopePath).Output()
	if err != nil {
		return false
	}
	prefix := strings.TrimSuffix(scopePath, "/") + "/"
	for _, line := range strings.Split(string(output), "\n") {
		clean, ok := cleanRepoPath(line)
		if ok && strings.HasPrefix(filepath.ToSlash(clean), prefix) {
			return true
		}
	}
	return false
}

func cleanRepoPath(rel string) (string, bool) {
	trimmed := strings.TrimSpace(rel)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return "", false
	}
	clean := filepath.Clean(trimmed)
	cleanSlash := filepath.ToSlash(clean)
	if clean == "." || clean == ".." || strings.HasPrefix(cleanSlash, "../") {
		return "", false
	}
	for _, part := range strings.Split(cleanSlash, "/") {
		if part == ".." {
			return "", false
		}
	}
	return clean, true
}

func hasGlobMagic(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

func scopePatternMatches(pattern string, rel string) bool {
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return rel == prefix || strings.HasPrefix(rel, prefix+"/")
	}
	matched, err := path.Match(pattern, rel)
	return err == nil && matched
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}

func sha256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + fmt.Sprintf("%x", digest)
}

func shortHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest[:6])
}

func provenanceError(options Options, message string, ref string) *resultdoc.CommandError {
	if options.ProvenanceIntegrityError != nil {
		return options.ProvenanceIntegrityError(message, ref)
	}
	return &resultdoc.CommandError{Type: "provenance_integrity_failed", Message: message, ExitCode: 9, Ref: ref}
}
