package assess

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
	memorydoc "github.com/Clyra-AI/relia/internal/memory"
	resultdoc "github.com/Clyra-AI/relia/internal/result"
	"github.com/Clyra-AI/relia/internal/yamlmini"
)

const objectType = "relia.risk_assessment"

type Options struct {
	SchemaVersion            string
	ArtifactContractError    func(string, string) *resultdoc.CommandError
	InternalError            func(string, error) *resultdoc.CommandError
	ProvenanceIntegrityError func(string, string) *resultdoc.CommandError
	RepoPathExists           func(string, string) bool
	YAMLFloat                func(float64) string
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
	ID                  string
	Kind                string
	Path                string
	Statement           string
	Confidence          float64
	ScopePaths          []string
	Citations           []RuleCitation
	EvidenceCount       int
	EvidenceExperiences []string
	ReviewGate          string
	ReviewDecision      string
}

type RuleCitation struct {
	URL     string
	PR      int
	Outcome string
}

func LoadRules(root string, options Options) ([]Rule, *resultdoc.CommandError) {
	patterns := []string{
		filepath.Join(root, "memory", "rules", "*.yaml"),
		filepath.Join(root, "memory", "rules", "*.yml"),
	}
	var paths []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, internalError(options, "could not inspect memory rule artifacts", err)
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)

	var rules []Rule
	for _, rulePath := range paths {
		rule, active, commandErr := ReadRule(root, rulePath, options)
		if commandErr != nil {
			return nil, commandErr
		}
		if active {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

func ReadRule(root string, rulePath string, options Options) (Rule, bool, *resultdoc.CommandError) {
	content, err := os.ReadFile(rulePath)
	if err != nil {
		return Rule{}, false, internalError(options, "could not read memory rule artifact", err)
	}
	rel, err := filepath.Rel(root, rulePath)
	if err != nil {
		rel = rulePath
	}
	rel = filepath.ToSlash(rel)
	document, parseErr := yamlmini.ParseDocument(string(content))
	if parseErr != nil {
		return Rule{}, false, artifactError(options, parseErr.Error(), rel)
	}
	if document.Scalars["status"].Value != "active" {
		return Rule{}, false, nil
	}
	if commandErr := ValidateActiveRuleIdentity(root, document, rel, options); commandErr != nil {
		return Rule{}, false, commandErr
	}
	confidence, err := strconv.ParseFloat(document.Scalars["confidence"].Value, 64)
	if err != nil {
		return Rule{}, false, artifactError(options, "memory rule confidence must be numeric", rel)
	}
	evidenceCount := len(yamlmini.ListValues(document, "evidence.experiences"))
	if evidenceCountScalar, ok := document.Scalars["evidence.count"]; ok {
		if parsed, err := strconv.Atoi(evidenceCountScalar.Value); err == nil && parsed > 0 {
			evidenceCount = parsed
		}
	}
	statement, ok := memorydoc.ResolvedRuleStatement(document, string(content))
	if !ok || strings.TrimSpace(statement) == "" {
		return Rule{}, false, artifactError(options, "memory rule statement is required", rel)
	}
	return Rule{
		ID:                  document.Scalars["id"].Value,
		Kind:                document.Scalars["kind"].Value,
		Path:                rel,
		Statement:           statement,
		Confidence:          confidence,
		ScopePaths:          yamlmini.ListValues(document, "scope.paths"),
		Citations:           RuleCitations(document),
		EvidenceCount:       evidenceCount,
		EvidenceExperiences: yamlmini.ListValues(document, "evidence.experiences"),
		ReviewGate:          activeRuleReviewGate(document),
		ReviewDecision:      activeRuleReviewDecision(document),
	}, true, nil
}

func ValidateActiveRuleIdentity(root string, document yamlmini.Document, rel string, options Options) *resultdoc.CommandError {
	required := []string{"object_type", "schema_version", "id", "kind", "status", "statement", "scope", "confidence", "evidence", "provenance", "review", "metadata"}
	for _, key := range required {
		if !yamlmini.HasPath(document, key) {
			return artifactError(options, "memory rule missing required key "+key, rel)
		}
	}
	if document.Scalars["object_type"].Value != "relia.memory_rule" {
		return artifactError(options, "memory rule object_type must be relia.memory_rule", configdoc.RefWithPath(rel, document.Scalars["object_type"]))
	}
	if document.Scalars["schema_version"].Value != options.SchemaVersion {
		return artifactError(options, "memory rule schema_version must be "+options.SchemaVersion, rel)
	}
	kind := document.Scalars["kind"].Value
	if kind != "avoid" && kind != "playbook" {
		return artifactError(options, "memory rule kind must be avoid or playbook", configdoc.RefWithPath(rel, document.Scalars["kind"]))
	}
	if kind == "playbook" && !HasPositivePlaybookEvidence(document) {
		return artifactError(options, "playbook memory rule must cite at least one fix_held or merged_clean provenance outcome", rel)
	}
	if len(document.Lists["scope.paths"]) == 0 && len(document.Lists["scope.signals"]) == 0 {
		return artifactError(options, "memory rule must declare at least one scope path or signal", rel)
	}
	for _, scopePath := range document.Lists["scope.paths"] {
		if !repoPathExists(options, root, scopePath.Value) {
			return artifactError(options, "memory rule scope path does not exist in the repo", configdoc.RefWithPath(rel, scopePath))
		}
	}
	confidence, err := strconv.ParseFloat(document.Scalars["confidence"].Value, 64)
	if err != nil {
		return artifactError(options, "memory rule confidence must be numeric", configdoc.RefWithPath(rel, document.Scalars["confidence"]))
	}
	reviewLabel, ok := document.Scalars["review.label"]
	if !ok {
		return artifactError(options, "memory rule missing required key review.label", rel)
	}
	if reviewLabel.Value != "accepted" {
		return artifactError(options, "active memory rule review.label must be accepted", configdoc.RefWithPath(rel, reviewLabel))
	}
	if reviewGate, ok := document.Scalars["review.gate"]; ok && reviewGate.Value != "human_review" {
		return artifactError(options, "memory rule review.gate is invalid", configdoc.RefWithPath(rel, reviewGate))
	}
	if reviewDecision, ok := document.Scalars["review.decision"]; ok && reviewDecision.Value != "approved" {
		return artifactError(options, "active memory rule review.decision must be approved", configdoc.RefWithPath(rel, reviewDecision))
	}
	statementOrigin, ok := document.Scalars["review.statement_origin"]
	if !ok {
		return artifactError(options, "memory rule missing required key review.statement_origin", rel)
	}
	switch statementOrigin.Value {
	case "llm_drafted", "cluster_summary", "human_authored":
	default:
		return artifactError(options, "memory rule review.statement_origin is invalid", configdoc.RefWithPath(rel, statementOrigin))
	}
	if len(document.Lists["evidence.experiences"]) == 0 {
		return artifactError(options, "memory rule must cite at least one experience", rel)
	}
	evidenceCount, ok := document.Scalars["evidence.count"]
	if !ok {
		return artifactError(options, "memory rule missing required key evidence.count", rel)
	}
	count, err := strconv.Atoi(evidenceCount.Value)
	if err != nil || count < 1 {
		return artifactError(options, "memory rule evidence.count must be at least 1", configdoc.RefWithPath(rel, evidenceCount))
	}
	contradictionsScalar, ok := document.Scalars["evidence.contradictions"]
	if !ok {
		return artifactError(options, "memory rule missing required key evidence.contradictions", rel)
	}
	contradictions, err := strconv.Atoi(contradictionsScalar.Value)
	if err != nil || contradictions < 0 {
		return artifactError(options, "memory rule evidence.contradictions must be at least 0", configdoc.RefWithPath(rel, contradictionsScalar))
	}
	provenanceEntries := document.Lists["provenance"]
	if len(provenanceEntries) == 0 {
		return artifactError(options, "memory rule must include at least one provenance entry", rel)
	}
	provenanceMaps := document.ListMaps["provenance"]
	if len(provenanceMaps) != len(provenanceEntries) {
		return artifactError(options, "memory rule provenance entries must include pr and outcome", rel)
	}
	for _, provenance := range provenanceMaps {
		pr, ok := provenance["pr"]
		if !ok {
			return artifactError(options, "memory rule provenance entry missing pr", rel)
		}
		prNumber, err := strconv.Atoi(pr.Value)
		if err != nil || prNumber < 1 {
			return artifactError(options, "memory rule provenance pr must be at least 1", configdoc.RefWithPath(rel, pr))
		}
		outcome, ok := provenance["outcome"]
		if !ok {
			return artifactError(options, "memory rule provenance entry missing outcome", rel)
		}
		switch outcome.Value {
		case "ci_failure", "revert", "review_correction", "fix_held", "merged_clean":
		default:
			return artifactError(options, "memory rule provenance outcome is invalid", configdoc.RefWithPath(rel, outcome))
		}
	}
	if commandErr := memorydoc.ValidateDraftedRuleCalibration(document, rel, confidence, count, contradictions, memoryValidationOptions(options)); commandErr != nil {
		return commandErr
	}
	return nil
}

func activeRuleReviewGate(document yamlmini.Document) string {
	if reviewGate, ok := document.Scalars["review.gate"]; ok {
		return reviewGate.Value
	}
	return "human_review"
}

func activeRuleReviewDecision(document yamlmini.Document) string {
	if reviewDecision, ok := document.Scalars["review.decision"]; ok {
		return reviewDecision.Value
	}
	return "approved"
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
			"rule_id":          rule.ID,
			"kind":             rule.Kind,
			"statement":        rule.Statement,
			"lifecycle_status": "active",
			"confidence":       rule.Confidence,
			"scope_paths":      append([]string(nil), rule.ScopePaths...),
			"citations":        servedCitations,
			"evidence_count":   RuleEvidenceCount(rule),
			"experience_ids":   append([]string(nil), rule.EvidenceExperiences...),
			"memory_source":    "active_rules_from_canonical_experience_records",
			"path":             rule.Path,
			"review_gate":      rule.ReviewGate,
			"review_decision":  rule.ReviewDecision,
		})
	}
	return data, nil
}

func BuildRiskAssessment(root string, inputRef string, content []byte, touchedPaths []string, rules []Rule, options Options, inputKinds ...string) (RiskAssessment, *resultdoc.CommandError) {
	inputKind := "diff"
	if len(inputKinds) > 0 && strings.TrimSpace(inputKinds[0]) != "" {
		inputKind = strings.TrimSpace(inputKinds[0])
	}
	matches := []RiskAssessmentMatch{}
	citations := []string{}
	matchedRules := []Rule{}
	highestAvoidConfidence := -1.0
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
		matchedRules = append(matchedRules, rule)
		citations = append(citations, servedCitations...)
		if rule.Kind != "playbook" && rule.Confidence > highestAvoidConfidence {
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
	pathCoverage, coverageStats := CoverageForPaths(root, touchedPaths, matchedRules)
	riskLevel := "no_coverage"
	if highestAvoidConfidence >= 0.75 {
		riskLevel = "match_high"
	} else if highestAvoidConfidence >= 0 {
		riskLevel = "match_medium"
	} else if coverageStats["coverage"] == "covered_clean" {
		riskLevel = "covered_clean"
	}
	metadata := map[string]any{
		"input_path":                inputRef,
		"input_kind":                inputKind,
		"diff_fingerprint":          sha256String(string(content)),
		"touched_paths":             touchedPaths,
		"coverage":                  coverageStats["coverage"],
		"coverage_stats":            coverageStats,
		"path_coverage":             pathCoverage,
		"experience_ids":            coverageStats["experience_ids"],
		"canonical_experience_refs": coverageStats["experience_ids"],
		"memory_source":             "active_rules_from_canonical_experience_records",
		"repo_relative_paths_only":  true,
		"redaction_status":          "customer_safe",
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

func CoverageForPaths(root string, touchedPaths []string, rules []Rule) ([]map[string]any, map[string]any) {
	normalizedPaths := uniqueStrings(touchedPaths)
	entries := make([]map[string]any, 0, len(normalizedPaths))
	summary := map[string]any{
		"coverage":                   "no_coverage",
		"touched_path_count":         len(normalizedPaths),
		"covered_path_count":         0,
		"covered_clean_count":        0,
		"covered_risky_count":        0,
		"no_coverage_path_count":     0,
		"no_coverage_count":          0,
		"matched_rule_count":         len(rules),
		"evidence_count":             EvidenceCountForRules(rules),
		"experience_ids":             ExperienceIDsForRules(rules),
		"experience_density":         0.0,
		"experience_density_source":  "active_memory_rule_evidence",
		"coverage_source":            "active_memory_rules",
		"canonical_record_source":    "memory_rule_evidence.experiences",
		"repo_relative_paths_only":   true,
		"out_of_distribution_signal": false,
	}
	if len(normalizedPaths) > 0 {
		summary["experience_density"] = float64(EvidenceCountForRules(rules)) / float64(len(normalizedPaths))
	}
	for _, touchedPath := range normalizedPaths {
		matched := RulesForTouchedPath(root, touchedPath, rules)
		coverage := CoverageLevelForRules(matched)
		evidenceCount := EvidenceCountForRules(matched)
		experienceIDs := ExperienceIDsForRules(matched)
		entry := map[string]any{
			"path":                touchedPath,
			"coverage":            coverage,
			"out_of_distribution": coverage == "no_coverage",
			"active_rule_count":   len(matched),
			"matched_rule_count":  len(matched),
			"matched_rule_ids":    RuleIDs(matched),
			"citations":           uniqueStrings(citationsFromRules(matched)),
			"evidence_count":      evidenceCount,
			"experience_ids":      experienceIDs,
			"experience_density":  float64(evidenceCount),
		}
		entries = append(entries, entry)
		switch coverage {
		case "covered_risky":
			summary["covered_path_count"] = summary["covered_path_count"].(int) + 1
			summary["covered_risky_count"] = summary["covered_risky_count"].(int) + 1
		case "covered_clean":
			summary["covered_path_count"] = summary["covered_path_count"].(int) + 1
			summary["covered_clean_count"] = summary["covered_clean_count"].(int) + 1
		default:
			summary["no_coverage_path_count"] = summary["no_coverage_path_count"].(int) + 1
			summary["no_coverage_count"] = summary["no_coverage_count"].(int) + 1
		}
	}
	summary["coverage"] = aggregateCoverage(summary)
	summary["out_of_distribution_signal"] = summary["no_coverage_path_count"].(int) > 0
	return entries, summary
}

func RulesForTouchedPath(root string, touchedPath string, rules []Rule) []Rule {
	var matched []Rule
	for _, rule := range rules {
		if RuleMatchesTouchedPath(root, rule, []string{touchedPath}) {
			matched = append(matched, rule)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Confidence == matched[j].Confidence {
			return matched[i].ID < matched[j].ID
		}
		return matched[i].Confidence > matched[j].Confidence
	})
	return matched
}

func CoverageLevelForRules(rules []Rule) string {
	if len(rules) == 0 {
		return "no_coverage"
	}
	for _, rule := range rules {
		if rule.Kind == "avoid" {
			return "covered_risky"
		}
	}
	return "covered_clean"
}

func EvidenceCountForRules(rules []Rule) int {
	count := 0
	for _, rule := range rules {
		count += RuleEvidenceCount(rule)
	}
	return count
}

func RuleEvidenceCount(rule Rule) int {
	if rule.EvidenceCount > 0 {
		return rule.EvidenceCount
	}
	return len(rule.EvidenceExperiences)
}

func ExperienceIDsForRules(rules []Rule) []string {
	var ids []string
	for _, rule := range rules {
		ids = append(ids, rule.EvidenceExperiences...)
	}
	return uniqueStrings(ids)
}

func RuleIDs(rules []Rule) []string {
	var ids []string
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	return uniqueStrings(ids)
}

func citationsFromRules(rules []Rule) []string {
	var citations []string
	for _, rule := range rules {
		citations = append(citations, ServedRuleCitationURLs(rule)...)
	}
	return citations
}

func aggregateCoverage(summary map[string]any) string {
	if summary["no_coverage_path_count"].(int) > 0 {
		return "no_coverage"
	}
	if summary["covered_risky_count"].(int) > 0 {
		return "covered_risky"
	}
	if summary["covered_clean_count"].(int) > 0 {
		return "covered_clean"
	}
	return "no_coverage"
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
	if unique == nil {
		return []string{}
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

func memoryValidationOptions(options Options) memorydoc.ValidationOptions {
	return memorydoc.ValidationOptions{
		SchemaVersion:         options.SchemaVersion,
		ArtifactContractError: options.ArtifactContractError,
		InternalError:         options.InternalError,
		RepoPathExists:        options.RepoPathExists,
		YAMLFloat:             options.YAMLFloat,
	}
}

func repoPathExists(options Options, root string, rel string) bool {
	if options.RepoPathExists != nil {
		return options.RepoPathExists(root, rel)
	}
	_, err := os.Stat(filepath.Join(root, rel))
	return err == nil
}

func artifactError(options Options, message string, ref string) *resultdoc.CommandError {
	if options.ArtifactContractError != nil {
		return options.ArtifactContractError(message, ref)
	}
	return &resultdoc.CommandError{Type: "artifact_contract_validation_failed", Message: message, Ref: ref}
}

func internalError(options Options, message string, err error) *resultdoc.CommandError {
	if options.InternalError != nil {
		return options.InternalError(message, err)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		message += ": " + err.Error()
	}
	return &resultdoc.CommandError{Type: "internal", Message: message}
}

func provenanceError(options Options, message string, ref string) *resultdoc.CommandError {
	if options.ProvenanceIntegrityError != nil {
		return options.ProvenanceIntegrityError(message, ref)
	}
	return &resultdoc.CommandError{Type: "provenance_integrity_failed", Message: message, ExitCode: 9, Ref: ref}
}
