package advise

import (
	"math"
	"strconv"
	"strings"
	"time"

	assessdoc "github.com/Clyra-AI/relia/internal/assess"
	configdoc "github.com/Clyra-AI/relia/internal/config"
)

const commentMarker = "relia-advisory:v1"

type PriorState struct {
	DiffFingerprint string
	GeneratedAt     time.Time
	RiskLevel       string
	SkipReason      string
}

func CommentDecision(settings configdoc.AdviseSettings, assessment assessdoc.RiskAssessment, diffFingerprint string, previousState PriorState, now time.Time) (bool, string) {
	if !settings.Enabled {
		return false, "advise_disabled"
	}
	if settings.MaxCommentsPerPR == 0 {
		return false, "comment_cap_zero"
	}
	previousFingerprint := previousState.DiffFingerprint
	if assessment.RiskLevel == "covered_clean" {
		if previousFingerprint != "" {
			if previousFingerprint == diffFingerprint && previousState.RiskLevel == "covered_clean" {
				return false, "unchanged_diff_fingerprint"
			}
			return true, ""
		}
		return false, "covered_clean"
	}
	if BelowMinConfidence(settings, assessment) {
		if previousFingerprint != "" {
			if previousFingerprint == diffFingerprint && previousState.RiskLevel == "below_min_confidence" {
				return false, "unchanged_diff_fingerprint"
			}
			return true, "below_min_confidence"
		}
		return false, "below_min_confidence"
	}
	if previousFingerprint != "" && previousFingerprint == diffFingerprint {
		currentRiskLevel := PublishedRiskLevel(assessment, "")
		if previousState.RiskLevel == "" || previousState.RiskLevel == currentRiskLevel {
			return false, "unchanged_diff_fingerprint"
		}
	}
	if previousFingerprint != "" && settings.ReassessDebounceMinutes > 0 && !previousState.GeneratedAt.IsZero() {
		debounceWindow := time.Duration(settings.ReassessDebounceMinutes) * time.Minute
		if now.Sub(previousState.GeneratedAt) < debounceWindow {
			return false, "reassess_debounce_window"
		}
	}
	return true, ""
}

func BelowMinConfidence(settings configdoc.AdviseSettings, assessment assessdoc.RiskAssessment) bool {
	return assessment.RiskLevel != "no_coverage" &&
		assessment.RiskLevel != "covered_clean" &&
		RiskConfidence(assessment) < settings.MinConfidence
}

func RiskConfidence(assessment assessdoc.RiskAssessment) float64 {
	if value, ok := assessment.Metadata["max_avoid_confidence"].(float64); ok {
		return value
	}
	return MaxConfidence(assessment)
}

func MaxConfidence(assessment assessdoc.RiskAssessment) float64 {
	maxConfidence := 0.0
	for _, match := range assessment.Matches {
		if match.Confidence > maxConfidence {
			maxConfidence = match.Confidence
		}
	}
	return maxConfidence
}

func CommentStrategy(settings configdoc.AdviseSettings) map[string]any {
	return map[string]any{
		"max_comments_per_pr":         settings.MaxCommentsPerPR,
		"update_in_place":             settings.UpdateInPlace,
		"reassess_debounce_minutes":   settings.ReassessDebounceMinutes,
		"min_confidence":              settings.MinConfidence,
		"comment_marker":              commentMarker,
		"unchanged_diff_skip_enabled": true,
	}
}

func PublishedRiskLevel(assessment assessdoc.RiskAssessment, skipReason string) string {
	if skipReason == "below_min_confidence" {
		return "below_min_confidence"
	}
	return assessment.RiskLevel
}

func RenderComment(assessment assessdoc.RiskAssessment, touchedPaths []string, diffFingerprint string, generatedAt time.Time, skipReason string) string {
	var builder strings.Builder
	builder.WriteString("<!-- " + commentMarker + " diff_fingerprint=" + diffFingerprint + " generated_at=" + generatedAt.UTC().Format(time.RFC3339) + " risk_level=" + PublishedRiskLevel(assessment, skipReason) + " -->\n")
	if skipReason == "below_min_confidence" {
		builder.WriteString("Relia advisory - current diff is below the advisory confidence threshold. Prior advisory cleared.")
		if len(assessment.Matches) > 0 {
			builder.WriteString(" Matched ")
			writeMatches(&builder, assessment.Matches)
		}
		builder.WriteString(".\n")
		return builder.String()
	}
	switch assessment.RiskLevel {
	case "no_coverage":
		builder.WriteString("Relia advisory - no prior active memory covers ")
		builder.WriteString(markdownInlineList(noCoveragePaths(assessment, touchedPaths), 3))
		builder.WriteString(". Suggest closer review.\n")
	case "covered_clean":
		builder.WriteString("Relia advisory - current diff is covered by active memory. Prior advisory cleared.")
		if len(assessment.Matches) > 0 {
			builder.WriteString(" Matched ")
			writeMatches(&builder, assessment.Matches)
		}
		writeCitations(&builder, assessment.Citations)
		builder.WriteString(".\n")
	default:
		builder.WriteString("Relia advisory - this change matches ")
		writeMatches(&builder, assessment.Matches)
		writeCitations(&builder, assessment.Citations)
		builder.WriteString(".\n")
	}
	return builder.String()
}

func noCoveragePaths(assessment assessdoc.RiskAssessment, fallback []string) []string {
	if assessment.Metadata == nil {
		return fallback
	}
	rawEntries, ok := assessment.Metadata["path_coverage"]
	if !ok {
		return fallback
	}
	paths := uncoveredPathsFromPathCoverage(rawEntries)
	if len(paths) == 0 {
		return fallback
	}
	return paths
}

func uncoveredPathsFromPathCoverage(rawEntries any) []string {
	var paths []string
	switch entries := rawEntries.(type) {
	case []map[string]any:
		for _, entry := range entries {
			if path, ok := uncoveredPathFromCoverageEntry(entry); ok {
				paths = append(paths, path)
			}
		}
	case []any:
		for _, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				continue
			}
			if path, ok := uncoveredPathFromCoverageEntry(entry); ok {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func uncoveredPathFromCoverageEntry(entry map[string]any) (string, bool) {
	coverage, _ := entry["coverage"].(string)
	if coverage != "no_coverage" {
		return "", false
	}
	path, _ := entry["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	return path, true
}

func writeMatches(builder *strings.Builder, matches []assessdoc.RiskAssessmentMatch) {
	for index, match := range matches {
		if index > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString("`" + match.RuleID + "`")
		builder.WriteString(" (confidence " + yamlFloat(match.Confidence) + ")")
	}
}

func writeCitations(builder *strings.Builder, citations []string) {
	if len(citations) == 0 {
		return
	}
	builder.WriteString(". Evidence: ")
	for index, citation := range citations {
		if index > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(citation)
	}
}

func markdownInlineList(values []string, limit int) string {
	if len(values) == 0 {
		return markdownCodeSpan("this change")
	}
	if limit > 0 && len(values) > limit {
		values = values[:limit]
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, markdownCodeSpan(value))
	}
	return strings.Join(parts, ", ")
}

func markdownCodeSpan(value string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, value)
	if cleaned == "" {
		cleaned = " "
	}
	delimiter := "`"
	for strings.Contains(cleaned, delimiter) {
		delimiter += "`"
	}
	if strings.Contains(cleaned, "`") || strings.HasPrefix(cleaned, " ") || strings.HasSuffix(cleaned, " ") {
		return delimiter + " " + cleaned + " " + delimiter
	}
	return delimiter + cleaned + delimiter
}

func yamlFloat(value float64) string {
	return strconv.FormatFloat(roundFloat(value, 4), 'f', -1, 64)
}

func roundFloat(value float64, places int) float64 {
	if places < 0 {
		return value
	}
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}
