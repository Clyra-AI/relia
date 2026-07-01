package distill

import (
	"encoding/json"
	"math"
	"strings"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
	"github.com/Clyra-AI/relia/internal/yamlmini"
)

type CostEstimate struct {
	InputTokensEstimated  int     `json:"input_tokens_estimated"`
	OutputTokensEstimated int     `json:"output_tokens_estimated"`
	TotalTokensEstimated  int     `json:"total_tokens_estimated"`
	EstimatedCostUSD      float64 `json:"estimated_cost_usd"`
	MaxCostUSDPerRun      float64 `json:"max_cost_usd_per_run"`
	CapStatus             string  `json:"cap_status"`
}

type providerAdapter interface {
	Surface() string
	RequestShape(configdoc.ProviderConfig) map[string]any
}

type openAICompatibleAdapter struct{}
type anthropicMessagesAdapter struct{}

func Provider(config yamlmini.Document) (string, string) {
	return configdoc.DistillProvider(config)
}

func NormalizeProvider(value string) string {
	return configdoc.NormalizeDistillProvider(value)
}

func BuildProviderPlan(config configdoc.ProviderConfig, records []ingestdoc.Record, embeddingMode string, sourceArtifacts []string, sourceDigest string) (map[string]any, *configdoc.Error) {
	if config.Provider == "none" {
		return map[string]any{
				"provider":                "none",
				"provider_call_attempted": false,
			}, &configdoc.Error{
				Kind:    configdoc.ErrorDependency,
				Message: "provider embeddings require distill.provider to be configured",
				Ref:     configdoc.DefaultFile,
			}
	}
	adapter, ok := providerAdapterFor(config.Provider)
	if !ok {
		return map[string]any{"provider": config.Provider}, &configdoc.Error{
			Kind:    configdoc.ErrorConfig,
			Message: "unsupported distill.provider " + config.Provider,
			Ref:     config.ProviderRef,
		}
	}
	cost := EstimateProviderCost(config, records)
	return map[string]any{
		"provider":                config.Provider,
		"adapter":                 adapter.Surface(),
		"model":                   config.Model,
		"base_url":                config.BaseURL,
		"credential_env":          config.CredentialEnv,
		"embedding_mode":          embeddingMode,
		"cost":                    cost,
		"request_shape":           adapter.RequestShape(config),
		"source_artifacts":        sourceArtifacts,
		"source_artifact_digest":  sourceDigest,
		"redacted_records_only":   true,
		"provider_call_attempted": false,
		"approval_required":       "model_provider_endpoint",
	}, nil
}

func EstimateProviderCost(config configdoc.ProviderConfig, records []ingestdoc.Record) CostEstimate {
	inputTokens := 0
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			inputTokens += 1
			continue
		}
		inputTokens += EstimatedTokensForBytes(len(encoded))
	}
	outputTokens := 96
	if len(records) > 0 {
		outputTokens += len(records) * 64
	}
	total := inputTokens + outputTokens
	estimate := (float64(inputTokens)/1000.0)*config.InputCostUSDPer1KTokens +
		(float64(outputTokens)/1000.0)*config.OutputCostUSDPer1KTokens
	estimate = roundFloat(estimate, 6)
	capStatus := "within_cap"
	if estimate > config.MaxCostUSDPerRun {
		capStatus = "exceeded"
	}
	return CostEstimate{
		InputTokensEstimated:  inputTokens,
		OutputTokensEstimated: outputTokens,
		TotalTokensEstimated:  total,
		EstimatedCostUSD:      estimate,
		MaxCostUSDPerRun:      config.MaxCostUSDPerRun,
		CapStatus:             capStatus,
	}
}

func EstimatedTokensForBytes(length int) int {
	if length <= 0 {
		return 0
	}
	return int(math.Ceil(float64(length) / 4.0))
}

func NoProviderCost() map[string]any {
	return map[string]any{
		"input_tokens_estimated":  0,
		"output_tokens_estimated": 0,
		"total_tokens_estimated":  0,
		"estimated_cost_usd":      0,
		"cap_status":              "not_applicable",
	}
}

func EmbeddingMode(config yamlmini.Document) string {
	if scalar, ok := config.Scalars["distill.embeddings"]; ok {
		return scalar.Value
	}
	return "signature"
}

func EffectiveEmbeddingMode(config yamlmini.Document, override string) string {
	if override != "" {
		return override
	}
	return EmbeddingMode(config)
}

func ReviewRequired(config yamlmini.Document) bool {
	if scalar, ok := config.Scalars["distill.review_required"]; ok {
		return scalar.Value != "false"
	}
	return true
}

func providerAdapterFor(provider string) (providerAdapter, bool) {
	switch NormalizeProvider(provider) {
	case "openai_compatible":
		return openAICompatibleAdapter{}, true
	case "anthropic":
		return anthropicMessagesAdapter{}, true
	default:
		return nil, false
	}
}

func (openAICompatibleAdapter) Surface() string { return "openai_compatible_chat_completions_http" }

func (openAICompatibleAdapter) RequestShape(config configdoc.ProviderConfig) map[string]any {
	return map[string]any{
		"method":            "POST",
		"url":               strings.TrimRight(config.BaseURL, "/") + "/chat/completions",
		"credential_header": "Authorization: Bearer ${" + config.CredentialEnv + "}",
		"redaction_posture": "redacted_experience_records_only",
		"response_contract": "draft rule statements only; evidence, scope, and confidence remain deterministic",
		"network_attempted": false,
	}
}

func (anthropicMessagesAdapter) Surface() string { return "anthropic_messages_http" }

func (anthropicMessagesAdapter) RequestShape(config configdoc.ProviderConfig) map[string]any {
	return map[string]any{
		"method":            "POST",
		"url":               strings.TrimRight(config.BaseURL, "/") + "/v1/messages",
		"credential_header": "x-api-key: ${" + config.CredentialEnv + "}",
		"redaction_posture": "redacted_experience_records_only",
		"response_contract": "draft rule statements only; evidence, scope, and confidence remain deterministic",
		"network_attempted": false,
	}
}

func roundFloat(value float64, places int) float64 {
	if places < 0 {
		return value
	}
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}
