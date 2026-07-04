package distill

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
)

type Rule struct {
	ID              string
	Kind            string
	Status          string
	Statement       string
	ScopePaths      []string
	ScopeSignals    []string
	Confidence      float64
	EvidenceCount   int
	Contradictions  int
	Experiences     []string
	Provenance      []RuleProvenance
	ReviewLabel     string
	ReviewGate      string
	ReviewDecision  string
	ReviewedBy      string
	DecisionRef     string
	MergedInto      string
	StatementOrigin string
	Metadata        RuleMetadata
}

type RuleProvenance struct {
	PR           int
	Outcome      string
	URL          string
	ExperienceID string
}

type RuleMetadata struct {
	ConfidenceLabel       string
	EvidenceCount         int
	RecencyWeight         float64
	Contradictions        int
	FlakeDiscount         float64
	ExtractionConfidence  float64
	DraftingModelWeight   float64
	HalfLifeDays          int
	LatestEvidenceAt      string
	OldestEvidenceAt      string
	AnchorRecordedAt      string
	LifecycleReason       string
	ClusterKey            string
	ClusterKeyHash        string
	ClusterProvenance     string
	SourceArtifacts       []string
	SourceArtifactDigest  string
	Provider              string
	EmbeddingMode         string
	ReviewRequired        bool
	DeterministicFallback bool
	MemorySource          string
	SourceRecordType      string
	ExcludedMemorySources []string
}

func RuleID(kind string, cluster Cluster) string {
	slug := slugifyRuleIDPart(cluster.Signal)
	if slug == "" {
		slug = "signature"
	}
	return fmt.Sprintf("%s-%s-%s", kind, slug, shortHash(kind+"\x00"+cluster.Key))
}

func RuleStatement(kind string, cluster Cluster, scopePaths []string) string {
	scope := "this scope"
	if len(scopePaths) > 0 {
		scope = strings.Join(scopePaths, ", ")
	}
	signal := cluster.Signal
	if signal == "" {
		signal = "the clustered signature"
	}
	if kind == "playbook" {
		return fmt.Sprintf("Prefer the held %s pattern in %s when this signature appears.", signal, scope)
	}
	return fmt.Sprintf("Avoid repeating %s in %s without addressing the prior failure evidence.", signal, scope)
}

func RenderRuleYAML(rule Rule) string {
	var builder strings.Builder
	builder.WriteString("object_type: relia.memory_rule\n")
	builder.WriteString("schema_version: \"1.0\"\n")
	builder.WriteString("id: " + YAMLScalar(rule.ID) + "\n")
	builder.WriteString("kind: " + YAMLScalar(rule.Kind) + "\n")
	builder.WriteString("status: " + YAMLScalar(rule.Status) + "\n")
	builder.WriteString("statement: " + YAMLScalar(rule.Statement) + "\n")
	builder.WriteString("scope:\n")
	writeYAMLStringList(&builder, "paths", rule.ScopePaths, 2)
	writeYAMLStringList(&builder, "signals", rule.ScopeSignals, 2)
	builder.WriteString("confidence: " + YAMLFloat(rule.Confidence) + "\n")
	builder.WriteString("evidence:\n")
	builder.WriteString("  count: " + strconv.Itoa(rule.EvidenceCount) + "\n")
	builder.WriteString("  contradictions: " + strconv.Itoa(rule.Contradictions) + "\n")
	writeYAMLStringList(&builder, "experiences", rule.Experiences, 2)
	builder.WriteString("provenance:\n")
	for _, provenance := range rule.Provenance {
		builder.WriteString("  - pr: " + strconv.Itoa(provenance.PR) + "\n")
		builder.WriteString("    outcome: " + YAMLScalar(provenance.Outcome) + "\n")
		if provenance.URL != "" {
			builder.WriteString("    url: " + YAMLScalar(provenance.URL) + "\n")
		}
		if provenance.ExperienceID != "" {
			builder.WriteString("    experience_id: " + YAMLScalar(provenance.ExperienceID) + "\n")
		}
	}
	builder.WriteString("review:\n")
	builder.WriteString("  label: " + YAMLScalar(rule.ReviewLabel) + "\n")
	builder.WriteString("  statement_origin: " + YAMLScalar(rule.StatementOrigin) + "\n")
	reviewGate := rule.ReviewGate
	if reviewGate == "" {
		reviewGate = "human_review"
	}
	builder.WriteString("  gate: " + YAMLScalar(reviewGate) + "\n")
	reviewDecision := rule.ReviewDecision
	if reviewDecision == "" {
		reviewDecision = "pending"
	}
	builder.WriteString("  decision: " + YAMLScalar(reviewDecision) + "\n")
	if rule.ReviewedBy != "" {
		builder.WriteString("  reviewed_by: " + YAMLScalar(rule.ReviewedBy) + "\n")
	}
	if rule.DecisionRef != "" {
		builder.WriteString("  decision_ref: " + YAMLScalar(rule.DecisionRef) + "\n")
	}
	if rule.MergedInto != "" {
		builder.WriteString("  merged_into: " + YAMLScalar(rule.MergedInto) + "\n")
	}
	builder.WriteString("metadata:\n")
	builder.WriteString("  confidence_label: " + YAMLScalar(rule.Metadata.ConfidenceLabel) + "\n")
	builder.WriteString("  lifecycle_reason: " + YAMLScalar(rule.Metadata.LifecycleReason) + "\n")
	builder.WriteString("  confidence_inputs:\n")
	builder.WriteString("    evidence_count: " + strconv.Itoa(rule.Metadata.EvidenceCount) + "\n")
	builder.WriteString("    recency_weight: " + YAMLFloat(rule.Metadata.RecencyWeight) + "\n")
	builder.WriteString("    contradictions: " + strconv.Itoa(rule.Metadata.Contradictions) + "\n")
	builder.WriteString("    flake_discount: " + YAMLFloat(rule.Metadata.FlakeDiscount) + "\n")
	builder.WriteString("    extraction_confidence: " + YAMLFloat(rule.Metadata.ExtractionConfidence) + "\n")
	builder.WriteString("    drafting_model_weight: " + YAMLFloat(rule.Metadata.DraftingModelWeight) + "\n")
	builder.WriteString("  decay:\n")
	builder.WriteString("    half_life_days: " + strconv.Itoa(rule.Metadata.HalfLifeDays) + "\n")
	builder.WriteString("    latest_evidence_at: " + YAMLScalar(rule.Metadata.LatestEvidenceAt) + "\n")
	builder.WriteString("    oldest_evidence_at: " + YAMLScalar(rule.Metadata.OldestEvidenceAt) + "\n")
	builder.WriteString("    anchor_recorded_at: " + YAMLScalar(rule.Metadata.AnchorRecordedAt) + "\n")
	builder.WriteString("  cluster:\n")
	builder.WriteString("    key: " + YAMLScalar(rule.Metadata.ClusterKey) + "\n")
	builder.WriteString("    key_hash: " + YAMLScalar(rule.Metadata.ClusterKeyHash) + "\n")
	builder.WriteString("    provenance: " + YAMLScalar(rule.Metadata.ClusterProvenance) + "\n")
	builder.WriteString("  source_artifact_digest: " + YAMLScalar(rule.Metadata.SourceArtifactDigest) + "\n")
	writeYAMLStringList(&builder, "source_artifacts", rule.Metadata.SourceArtifacts, 2)
	builder.WriteString("  provider: " + YAMLScalar(rule.Metadata.Provider) + "\n")
	builder.WriteString("  embedding_mode: " + YAMLScalar(rule.Metadata.EmbeddingMode) + "\n")
	builder.WriteString("  review_required: " + strconv.FormatBool(rule.Metadata.ReviewRequired) + "\n")
	builder.WriteString("  deterministic_fallback: " + strconv.FormatBool(rule.Metadata.DeterministicFallback) + "\n")
	builder.WriteString("  memory_source: " + YAMLScalar(rule.Metadata.MemorySource) + "\n")
	builder.WriteString("  source_record_type: " + YAMLScalar(rule.Metadata.SourceRecordType) + "\n")
	writeYAMLStringList(&builder, "excluded_memory_sources", rule.Metadata.ExcludedMemorySources, 2)
	builder.WriteString("  generated_by: relia distill\n")
	builder.WriteString("  redaction_status: applied\n")
	return builder.String()
}

func YAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, "\n\r#") ||
		strings.Contains(value, ": ") ||
		strings.HasPrefix(value, "-") ||
		strings.HasPrefix(value, "{") ||
		strings.HasPrefix(value, "[") ||
		strings.HasSuffix(value, ":") ||
		value == "true" ||
		value == "false" {
		return strconv.Quote(value)
	}
	return value
}

func YAMLFloat(value float64) string {
	return strconv.FormatFloat(roundFloat(value, 4), 'f', -1, 64)
}

func writeYAMLStringList(builder *strings.Builder, key string, values []string, indent int) {
	prefix := strings.Repeat(" ", indent)
	builder.WriteString(prefix + key + ":\n")
	for _, value := range values {
		builder.WriteString(prefix + "  - " + YAMLScalar(value) + "\n")
	}
}

func slugifyRuleIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		keep := false
		switch {
		case r >= 'a' && r <= 'z':
			keep = true
		case r >= '0' && r <= '9':
			keep = true
		}
		if keep {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func shortHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest)[:12]
}
