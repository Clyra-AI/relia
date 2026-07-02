package distill

import "testing"

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
