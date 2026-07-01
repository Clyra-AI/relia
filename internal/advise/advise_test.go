package advise

import (
	"strings"
	"testing"
	"time"

	assessdoc "github.com/Clyra-AI/relia/internal/assess"
	configdoc "github.com/Clyra-AI/relia/internal/config"
)

func TestCommentDecisionClearsPriorBelowMinimumAdvisory(t *testing.T) {
	settings := configdoc.AdviseSettings{
		Enabled:          true,
		MaxCommentsPerPR: 1,
		MinConfidence:    0.6,
	}
	assessment := assessdoc.RiskAssessment{
		RiskLevel: "match_medium",
		Matches: []assessdoc.RiskAssessmentMatch{
			{RuleID: "billing-low-confidence", Confidence: 0.55},
		},
		Metadata: map[string]any{"max_avoid_confidence": 0.55},
	}
	previous := PriorState{
		DiffFingerprint: "sha256:previous",
		GeneratedAt:     time.Now().UTC().Add(-time.Hour),
		RiskLevel:       "match_high",
	}

	shouldComment, skipReason := CommentDecision(settings, assessment, "sha256:current", previous, time.Now().UTC())

	if !shouldComment || skipReason != "below_min_confidence" {
		t.Fatalf("CommentDecision = (%v, %q), want clear below-min advisory", shouldComment, skipReason)
	}
}

func TestRenderCommentEscapesTouchedPathCodeSpans(t *testing.T) {
	assessment := assessdoc.RiskAssessment{
		RiskLevel: "no_coverage",
		Metadata:  map[string]any{},
	}

	comment := RenderComment(assessment, []string{"packages/weird/`@here.py"}, "sha256:abc", time.Unix(0, 0).UTC(), "")

	if strings.Contains(comment, "`packages/weird/`@here.py`") {
		t.Fatalf("comment rendered unsafe single-backtick span:\n%s", comment)
	}
	if !strings.Contains(comment, "`` packages/weird/`@here.py ``") {
		t.Fatalf("comment missing escaped code span:\n%s", comment)
	}
	if !strings.Contains(comment, "risk_level=no_coverage") {
		t.Fatalf("comment missing risk level marker:\n%s", comment)
	}
}
