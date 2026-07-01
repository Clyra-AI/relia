package distill

import (
	"testing"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
	"github.com/Clyra-AI/relia/internal/yamlmini"
)

func TestBuildProviderPlanOpenAICompatible(t *testing.T) {
	config := configdoc.ProviderConfig{
		Provider:                 "openai_compatible",
		Model:                    "gpt-test",
		BaseURL:                  "https://api.example.test/v1/",
		CredentialEnv:            "RELIA_TEST_API_KEY",
		MaxCostUSDPerRun:         1,
		InputCostUSDPer1KTokens:  0.01,
		OutputCostUSDPer1KTokens: 0.02,
		ProviderRef:              "relia.yaml:10",
	}
	records := []ingestdoc.Record{{
		ExperienceID: "exp_1",
		Outcome:      ingestdoc.Outcome{Kind: "failed"},
		Metadata:     map[string]any{"message": "sample failure"},
	}}

	plan, configErr := BuildProviderPlan(config, records, "provider", []string{".relia/experiences/shard.jsonl"}, "sha256:abc")
	if configErr != nil {
		t.Fatalf("BuildProviderPlan returned error: %v", configErr)
	}

	if plan["adapter"] != "openai_compatible_chat_completions_http" {
		t.Fatalf("unexpected adapter: %v", plan["adapter"])
	}
	if plan["approval_required"] != "model_provider_endpoint" {
		t.Fatalf("unexpected approval gate: %v", plan["approval_required"])
	}
	cost, ok := plan["cost"].(CostEstimate)
	if !ok {
		t.Fatalf("cost has type %T, want CostEstimate", plan["cost"])
	}
	if cost.CapStatus != "within_cap" || cost.TotalTokensEstimated <= 0 {
		t.Fatalf("unexpected cost estimate: %+v", cost)
	}
	shape, ok := plan["request_shape"].(map[string]any)
	if !ok {
		t.Fatalf("request_shape has type %T, want map", plan["request_shape"])
	}
	if shape["url"] != "https://api.example.test/v1/chat/completions" {
		t.Fatalf("unexpected request URL: %v", shape["url"])
	}
	if shape["network_attempted"] != false {
		t.Fatalf("network_attempted = %v, want false", shape["network_attempted"])
	}
}

func TestBuildProviderPlanNoProviderReturnsDependency(t *testing.T) {
	plan, configErr := BuildProviderPlan(configdoc.ProviderConfig{Provider: "none"}, nil, "provider", nil, "")
	if configErr == nil {
		t.Fatal("expected dependency error")
	}
	if configErr.Kind != configdoc.ErrorDependency {
		t.Fatalf("error kind = %s, want %s", configErr.Kind, configdoc.ErrorDependency)
	}
	if plan["provider_call_attempted"] != false {
		t.Fatalf("provider_call_attempted = %v, want false", plan["provider_call_attempted"])
	}
}

func TestEstimateProviderCostMarksExceeded(t *testing.T) {
	config := configdoc.ProviderConfig{
		MaxCostUSDPerRun:         0.000001,
		InputCostUSDPer1KTokens:  1,
		OutputCostUSDPer1KTokens: 1,
	}
	cost := EstimateProviderCost(config, []ingestdoc.Record{{ExperienceID: "exp_1"}})
	if cost.CapStatus != "exceeded" {
		t.Fatalf("cap status = %s, want exceeded; cost=%+v", cost.CapStatus, cost)
	}
}

func TestDistillSettingsFromYAML(t *testing.T) {
	document, err := yamlmini.ParseDocument(`distill:
  provider: openai-compatible
  embeddings: local
  review_required: false
`)
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	provider, _ := Provider(document)
	if provider != "openai_compatible" {
		t.Fatalf("provider = %q, want openai_compatible", provider)
	}
	if EmbeddingMode(document) != "local" {
		t.Fatalf("embedding mode = %q, want local", EmbeddingMode(document))
	}
	if EffectiveEmbeddingMode(document, "provider") != "provider" {
		t.Fatalf("effective embedding override not honored")
	}
	if ReviewRequired(document) {
		t.Fatalf("review required = true, want false")
	}
}
