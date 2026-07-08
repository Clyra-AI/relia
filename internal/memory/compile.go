package memory

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	ManagedBeginMarker = "<!-- relia:begin (generated; edit rules in memory/rules/, not here) -->"
	ManagedEndMarker   = "<!-- relia:end -->"
)

func containsManagedMarker(value string) bool {
	return strings.Contains(value, ManagedBeginMarker) || strings.Contains(value, ManagedEndMarker)
}

type CompileOptions struct {
	SchemaVersion string
	ReliaVersion  string
	MaxRules      int
}

type CompiledContext struct {
	ObjectType    string                `json:"object_type"`
	SchemaVersion string                `json:"schema_version"`
	ContextID     string                `json:"context_id"`
	Target        string                `json:"target"`
	Rules         []CompiledContextRule `json:"rules"`
	Metadata      map[string]any        `json:"metadata"`
}

type CompiledContextRule struct {
	RuleID        string `json:"rule_id"`
	Status        string `json:"status"`
	CitationCount int    `json:"citation_count"`
}

func SelectCompiledRules(rules []RuleSummary, maxRules int) []RuleSummary {
	if maxRules <= 0 {
		maxRules = 25
	}
	var active []RuleSummary
	for _, rule := range rules {
		if rule.Status != "active" {
			continue
		}
		if rule.ReviewLabel != "accepted" || rule.ReviewGate != "human_review" || rule.ReviewDecision != "approved" {
			continue
		}
		active = append(active, rule)
	}
	sort.Slice(active, func(i, j int) bool {
		left := parsedConfidence(active[i].Confidence)
		right := parsedConfidence(active[j].Confidence)
		if left == right {
			return active[i].ID < active[j].ID
		}
		return left > right
	})
	if len(active) > maxRules {
		active = active[:maxRules]
	}
	return active
}

func RenderManagedBlock(rules []RuleSummary, options CompileOptions) string {
	options = defaultCompileOptions(options)
	var builder strings.Builder
	builder.WriteString(ManagedBeginMarker)
	builder.WriteString("\n")
	builder.WriteString("Relia managed context (schema_version=" + options.SchemaVersion + "; relia_version=" + options.ReliaVersion + "; source=memory/rules).\n")
	builder.WriteString("Do not edit this block directly; edit reviewed rules in `memory/rules/` and run `relia compile`.\n")
	builder.WriteString("Non-MCP agents should treat only the active accepted rules below as Relia memory.\n\n")
	if len(rules) == 0 {
		builder.WriteString("No active accepted Relia memory rules are available.\n")
		builder.WriteString(ManagedEndMarker)
		return builder.String()
	}
	for _, rule := range rules {
		receipts := provenanceReceiptLines(rule.Provenance)
		citationLabel := "citations"
		if len(receipts) == 1 {
			citationLabel = "citation"
		}
		builder.WriteString("- `" + rule.ID + "`")
		builder.WriteString(" (" + rule.Kind + ", confidence " + rule.Confidence + ", " + strconv.Itoa(len(receipts)) + " " + citationLabel + "): ")
		builder.WriteString(rule.Statement)
		builder.WriteString("\n")
		if len(receipts) > 0 {
			builder.WriteString("  receipts: " + strings.Join(receipts, "; ") + "\n")
		}
		builder.WriteString("  source: `" + rule.Path + "`\n")
	}
	builder.WriteString(ManagedEndMarker)
	return builder.String()
}

func UpsertManagedBlock(content string, block string) (string, bool, error) {
	beginCount := strings.Count(content, ManagedBeginMarker)
	endCount := strings.Count(content, ManagedEndMarker)
	if beginCount != endCount {
		return "", false, fmt.Errorf("managed marker mismatch: found %d begin markers and %d end markers", beginCount, endCount)
	}
	if beginCount > 1 {
		return "", false, fmt.Errorf("managed marker block must appear at most once")
	}
	if beginCount == 0 {
		prefix := content
		if strings.TrimSpace(prefix) == "" {
			next := block + "\n"
			return next, next != content, nil
		}
		if !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
		}
		next := prefix + "\n" + block + "\n"
		return next, next != content, nil
	}
	begin := strings.Index(content, ManagedBeginMarker)
	end := strings.Index(content, ManagedEndMarker)
	if begin < 0 || end < 0 || end < begin {
		return "", false, fmt.Errorf("managed marker order is invalid")
	}
	end += len(ManagedEndMarker)
	next := content[:begin] + block + content[end:]
	return next, next != content, nil
}

func CompiledContextForTarget(target string, rules []RuleSummary, options CompileOptions) CompiledContext {
	options = defaultCompileOptions(options)
	contextRules := make([]CompiledContextRule, 0, len(rules))
	for _, rule := range rules {
		contextRules = append(contextRules, CompiledContextRule{
			RuleID:        rule.ID,
			Status:        "active",
			CitationCount: len(provenanceReceiptLines(rule.Provenance)),
		})
	}
	return CompiledContext{
		ObjectType:    "relia.compiled_context",
		SchemaVersion: options.SchemaVersion,
		ContextID:     compiledContextID(target, rules),
		Target:        target,
		Rules:         contextRules,
		Metadata: map[string]any{
			"source":                  "memory/rules",
			"managed_begin_marker":    ManagedBeginMarker,
			"managed_end_marker":      ManagedEndMarker,
			"active_memory_only":      true,
			"emitted_rule_count":      len(contextRules),
			"max_rules":               options.MaxRules,
			"hosted_service_required": false,
			"live_network_required":   false,
		},
	}
}

func CompiledAgentAccessBoundary() map[string]any {
	return map[string]any{
		"active_memory_only":       true,
		"served_statuses":          []string{"active"},
		"required_review_label":    "accepted",
		"required_review_gate":     "human_review",
		"required_review_decision": "approved",
		"citation_required":        true,
		"hosted_service_required":  false,
		"live_network_required":    false,
		"access_surface":           "managed AGENTS.md and CLAUDE.md block for non-MCP agents",
	}
}

func defaultCompileOptions(options CompileOptions) CompileOptions {
	if options.SchemaVersion == "" {
		options.SchemaVersion = "1.0"
	}
	if options.ReliaVersion == "" {
		options.ReliaVersion = "0.0.0-dev"
	}
	if options.MaxRules <= 0 {
		options.MaxRules = 25
	}
	return options
}

func parsedConfidence(value string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return -1
	}
	return parsed
}

func compiledContextID(target string, rules []RuleSummary) string {
	var builder strings.Builder
	builder.WriteString(target)
	for _, rule := range rules {
		builder.WriteString("\x00")
		builder.WriteString(rule.ID)
		builder.WriteString("\x00")
		builder.WriteString(rule.Confidence)
		for _, receipt := range rule.Provenance {
			builder.WriteString("\x00")
			builder.WriteString(receipt.URL)
		}
	}
	digest := sha256.Sum256([]byte(builder.String()))
	return fmt.Sprintf("sha256:%x", digest)
}
