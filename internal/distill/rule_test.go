package distill

import (
	"strings"
	"testing"
)

func TestRuleIDUsesSignalSlugAndStableHash(t *testing.T) {
	cluster := Cluster{Key: "class\x00go\x00build", Signal: "go test ./..."}

	got := RuleID("avoid", cluster)
	want := "avoid-go-test-" + shortHash("avoid\x00"+cluster.Key)
	if got != want {
		t.Fatalf("RuleID = %q, want %q", got, want)
	}
}

func TestRuleIDFallsBackToSignatureSlug(t *testing.T) {
	cluster := Cluster{Key: "id\x00sig-generated", Signal: "!!!"}

	got := RuleID("playbook", cluster)
	want := "playbook-signature-" + shortHash("playbook\x00"+cluster.Key)
	if got != want {
		t.Fatalf("RuleID = %q, want %q", got, want)
	}
}

func TestRuleStatementIncludesKindSignalAndScope(t *testing.T) {
	cluster := Cluster{Signal: "go vet"}

	avoid := RuleStatement("avoid", cluster, []string{"cmd/relia/main.go"})
	if avoid != "Avoid repeating go vet in cmd/relia/main.go without addressing the prior failure evidence." {
		t.Fatalf("avoid statement = %q", avoid)
	}
	playbook := RuleStatement("playbook", cluster, nil)
	if playbook != "Prefer the held go vet pattern in this scope when this signature appears." {
		t.Fatalf("playbook statement = %q", playbook)
	}
}

func TestYAMLScalarQuotesColonSpace(t *testing.T) {
	if got := YAMLScalar("build: lint"); got != `"build: lint"` {
		t.Fatalf("YAMLScalar = %q, want quoted colon-space scalar", got)
	}
}

func TestRenderRuleYAMLIncludesStableMetadata(t *testing.T) {
	rule := Rule{
		ID:              "avoid-build-lint",
		Kind:            "avoid",
		Status:          "candidate",
		Statement:       "Avoid build: lint without checking prior failures.",
		ScopePaths:      []string{"cmd/relia/main.go"},
		ScopeSignals:    []string{"build: lint"},
		Confidence:      0.67891,
		EvidenceCount:   2,
		Contradictions:  1,
		Experiences:     []string{"exp-1", "exp-2"},
		Provenance:      []RuleProvenance{{PR: 42, Outcome: "failure", URL: "https://github.com/Clyra-AI/relia/pull/42", ExperienceID: "exp-1"}},
		ReviewLabel:     "suggested",
		ReviewGate:      "human_review",
		ReviewDecision:  "pending",
		ReviewedBy:      "maintainer",
		DecisionRef:     "relia review",
		StatementOrigin: "cluster_summary",
		Metadata: RuleMetadata{
			ConfidenceLabel:       "medium",
			EvidenceCount:         2,
			RecencyWeight:         0.87654,
			Contradictions:        1,
			FlakeDiscount:         0.12555,
			ExtractionConfidence:  0.95555,
			DraftingModelWeight:   0,
			HalfLifeDays:          90,
			LatestEvidenceAt:      "2026-06-02T00:00:00Z",
			OldestEvidenceAt:      "2026-06-01T00:00:00Z",
			AnchorRecordedAt:      "2026-06-03T00:00:00Z",
			LifecycleReason:       "human review required before activation",
			ClusterKey:            "class|go|build",
			ClusterKeyHash:        "abc123",
			ClusterProvenance:     "signature_only",
			SourceArtifacts:       []string{".relia/experiences/2026-06.jsonl"},
			SourceArtifactDigest:  "sha256:abc",
			Provider:              "none",
			EmbeddingMode:         "signature",
			ReviewRequired:        true,
			DeterministicFallback: true,
			MemorySource:          "verified_outcome_events",
			SourceRecordType:      "relia.experience_record",
			ExcludedMemorySources: []string{"agent_self_report", "agent_reflection"},
		},
	}

	got := RenderRuleYAML(rule)
	for _, want := range []string{
		`statement: "Avoid build: lint without checking prior failures."`,
		`    recency_weight: 0.8765`,
		`    flake_discount: 0.1256`,
		`    extraction_confidence: 0.9556`,
		`  - .relia/experiences/2026-06.jsonl`,
		`  deterministic_fallback: true`,
		`    url: https://github.com/Clyra-AI/relia/pull/42`,
		`  gate: human_review`,
		`  decision: pending`,
		`  reviewed_by: maintainer`,
		`  decision_ref: relia review`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered YAML missing %q:\n%s", want, got)
		}
	}
}
