package backtest

import (
	"reflect"
	"testing"
	"time"
)

func TestIsFlakeDiscountedUsesExplicitAndHeuristicDiscounts(t *testing.T) {
	explicit := recurrenceExperienceForTest("exp-explicit", 12, "sig-a", []string{"cmd/app.go"}, time.Time{})
	explicit.Record.FlakeDiscount = 0.5
	heuristic := recurrenceExperienceForTest("exp-heuristic", 13, "sig-a", []string{"cmd/app.go"}, time.Time{})
	plain := recurrenceExperienceForTest("exp-plain", 14, "sig-a", []string{"cmd/app.go"}, time.Time{})

	heuristics := map[string]string{
		"exp-heuristic": "heuristic reason",
	}

	if !IsFlakeDiscounted(explicit, heuristics) {
		t.Fatalf("explicit flake discount was not detected")
	}
	if !IsFlakeDiscounted(heuristic, heuristics) {
		t.Fatalf("heuristic flake discount was not detected")
	}
	if IsFlakeDiscounted(plain, heuristics) {
		t.Fatalf("plain record was incorrectly discounted")
	}
}

func TestBuildFlakeDiscountShapesHeuristicEvidence(t *testing.T) {
	record := recurrenceExperienceForTest("exp-3", 30, "sig-a", []string{"cmd/app.go"}, time.Time{})
	prior := recurrenceExperienceForTest("exp-1", 10, "sig-a", []string{"cmd/app.go"}, time.Time{})
	later := recurrenceExperienceForTest("exp-2", 20, "sig-a", []string{"cmd/app.go"}, time.Time{})
	human := recurrenceExperienceForTest("exp-human", 25, "sig-a", []string{"cmd/app.go"}, time.Time{})
	human.Record.Attribution.ActorKind = "human"
	unrelated := recurrenceExperienceForTest("exp-other", 40, "sig-b", []string{"cmd/app.go"}, time.Time{})

	discount := BuildFlakeDiscount(record, []Experience{later, record, human, unrelated, prior}, map[string]string{
		"exp-3": "heuristic reason",
	})

	if discount.ExperienceID != "exp-3" || discount.PR != 30 || discount.SignatureID != "sig-a" {
		t.Fatalf("discount identity = %#v, want exp-3 PR 30 sig-a", discount)
	}
	if discount.FlakeDiscount != 1 || discount.Reason != "heuristic reason" || !discount.ExcludedFromERR {
		t.Fatalf("discount policy = %#v, want heuristic full exclusion", discount)
	}
	if !reflect.DeepEqual(discount.SupportingPRs, []int{10, 20}) {
		t.Fatalf("supporting PRs = %#v, want sorted agent recurrence PRs", discount.SupportingPRs)
	}
	if !reflect.DeepEqual(discount.SupportingRefs, []string{"events.jsonl:20", "events.jsonl:10"}) {
		t.Fatalf("supporting refs = %#v, want first-seen unique refs", discount.SupportingRefs)
	}
}

func TestBuildFlakeDiscountUsesExplicitDiscountReasonAndRounding(t *testing.T) {
	record := recurrenceExperienceForTest("exp-3", 30, "sig-a", []string{"cmd/app.go"}, time.Time{})
	record.Record.FlakeDiscount = 0.333333

	discount := BuildFlakeDiscount(record, []Experience{record}, nil)

	if discount.FlakeDiscount != 0.3333 {
		t.Fatalf("FlakeDiscount = %v, want rounded explicit value", discount.FlakeDiscount)
	}
	if discount.Reason != "Discounted as flaky because the experience record carries an explicit flake_discount." {
		t.Fatalf("Reason = %q, want explicit discount reason", discount.Reason)
	}
}
