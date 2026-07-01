package config

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Clyra-AI/relia/internal/yamlmini"
)

const DefaultFile = "relia.yaml"

type Finding struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Ref     string `json:"ref,omitempty"`
}

type ErrorKind string

const (
	ErrorConfig           ErrorKind = "config"
	ErrorArtifactContract ErrorKind = "artifact_contract"
	ErrorRedactionSafety  ErrorKind = "redaction_safety"
	ErrorDependency       ErrorKind = "dependency"
	ErrorInternal         ErrorKind = "internal"
)

type Error struct {
	Kind    ErrorKind
	Message string
	Ref     string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

type ValidationOptions struct {
	SchemaVersion     string
	ReliaVersion      string
	EmbeddingOverride string
}

type ProviderConfig struct {
	Provider                 string
	Model                    string
	BaseURL                  string
	CredentialEnv            string
	MaxCostUSDPerRun         float64
	InputCostUSDPer1KTokens  float64
	OutputCostUSDPer1KTokens float64
	ProviderRef              string
}

type AdviseSettings struct {
	Enabled                 bool
	MaxCommentsPerPR        int
	UpdateInPlace           bool
	ReassessDebounceMinutes int
	MinConfidence           float64
}

type LocalModelManifest struct {
	ModelID        string `json:"model_id"`
	Version        string `json:"version"`
	SourceURL      string `json:"source_url"`
	License        string `json:"license"`
	Digest         string `json:"digest"`
	CachePath      string `json:"cache_path"`
	UpdatePolicy   string `json:"update_policy"`
	RollbackPolicy string `json:"rollback_policy"`
	Status         string `json:"status,omitempty"`
}

func DefaultYAML(schemaVersion string, reliaVersion string) string {
	return fmt.Sprintf(`version: 1

artifacts:
  schema_version: "%s"
  relia_version: "%s"
  root: .relia
  commit_experiences: false

repo:
  provider: github
  remote: origin
  scopes: []

attribution:
  agent_authors: []
  coauthor_trailers:
    - Claude
    - Claude Code
  pr_labels:
    - agent-authored
  uncertain: exclude

outcomes:
  checks:
    required: []

privacy:
  local_only: true
  send_code: false
  send_diffs: false
  send_logs: false
  send_experience_records: false
  share_scope: private

redaction:
  schema_version: "%s"
  entropy_scan: true
  fail_closed: true
  standard_token_shapes: true

distill:
  embeddings: signature
  provider: none
  model: ""
  base_url: ""
  credential_env: ""
  max_cost_usd_per_run: 0
  input_cost_usd_per_1k_tokens: 0
  output_cost_usd_per_1k_tokens: 0
  review_required: true

models:
  local_manifest: .relia/models/manifest.json

serve:
  advisory_only: true

advise:
  enabled: true
  max_comments_per_pr: 1
  update_in_place: true
  reassess_debounce_minutes: 10
  min_confidence: 0.6

gate:
  enabled: false
`, schemaVersion, reliaVersion, schemaVersion)
}

func Ref(defaultPath string, scalar yamlmini.Scalar) string {
	return RefWithPath(defaultPath, scalar)
}

func RefWithPath(path string, scalar yamlmini.Scalar) string {
	if scalar.Line <= 0 {
		return path
	}
	return fmt.Sprintf("%s:%d", path, scalar.Line)
}

func PathRef(defaultPath string, document yamlmini.Document, path string) string {
	if scalar, ok := document.Scalars[path]; ok {
		return Ref(defaultPath, scalar)
	}
	if scalar, ok := document.Containers[path]; ok {
		return Ref(defaultPath, scalar)
	}
	if scalars := document.Lists[path]; len(scalars) > 0 {
		return Ref(defaultPath, scalars[0])
	}
	prefix := path + "."
	bestLine := 0
	for _, collection := range []map[string]yamlmini.Scalar{document.Scalars, document.Containers} {
		for key, scalar := range collection {
			if strings.HasPrefix(key, prefix) && (bestLine == 0 || scalar.Line < bestLine) {
				bestLine = scalar.Line
			}
		}
	}
	if bestLine > 0 {
		return Ref(defaultPath, yamlmini.Scalar{Line: bestLine})
	}
	return defaultPath
}

func Read(root string) (yamlmini.Document, *Error) {
	content, err := os.ReadFile(filepath.Join(root, DefaultFile))
	if err != nil {
		return yamlmini.Document{}, internalError("could not read relia.yaml", err)
	}
	document, parseErr := yamlmini.ParseDocument(string(content))
	if parseErr != nil {
		return yamlmini.Document{}, configError(parseErr.Error())
	}
	return document, nil
}

func Validate(root string, options ValidationOptions) ([]Finding, *Error) {
	document, configErr := Read(root)
	if configErr != nil {
		return nil, configErr
	}
	return ValidateDocument(root, document, options)
}

func ValidateDocument(root string, document yamlmini.Document, options ValidationOptions) ([]Finding, *Error) {
	requiredExact := map[string]string{
		"version":                         "1",
		"artifacts.schema_version":        options.SchemaVersion,
		"artifacts.relia_version":         options.ReliaVersion,
		"artifacts.root":                  ".relia",
		"artifacts.commit_experiences":    "false",
		"privacy.local_only":              "true",
		"privacy.send_code":               "false",
		"privacy.send_diffs":              "false",
		"privacy.send_logs":               "false",
		"privacy.send_experience_records": "false",
		"privacy.share_scope":             "private",
		"redaction.schema_version":        options.SchemaVersion,
		"redaction.entropy_scan":          "true",
		"redaction.fail_closed":           "true",
		"redaction.standard_token_shapes": "true",
		"models.local_manifest":           ".relia/models/manifest.json",
		"serve.advisory_only":             "true",
	}
	for key, want := range requiredExact {
		scalar, ok := document.Scalars[key]
		if !ok {
			return nil, configError(fmt.Sprintf("relia.yaml missing required key %s", key))
		}
		if scalar.Value != want {
			switch key {
			case "artifacts.commit_experiences", "privacy.local_only", "privacy.send_code", "privacy.send_diffs", "privacy.send_logs", "privacy.send_experience_records", "privacy.share_scope":
				return nil, artifactContractError(fmt.Sprintf("%s must be %s for the MVP artifact contract", key, want), Ref(DefaultFile, scalar))
			case "redaction.entropy_scan", "redaction.fail_closed", "redaction.standard_token_shapes":
				return nil, redactionSafetyError(fmt.Sprintf("%s must be %s", key, want), Ref(DefaultFile, scalar))
			default:
				return nil, configError(fmt.Sprintf("%s must be %s", key, want))
			}
		}
	}

	var warnings []Finding
	gateEnabled, ok := document.Scalars["gate.enabled"]
	if !ok {
		return nil, configError("relia.yaml missing required key gate.enabled")
	}
	switch gateEnabled.Value {
	case "false":
		if gateLimit, ok := document.Scalars["gate.max_error_recurrence_rate"]; ok {
			warnings = append(warnings, Finding{
				Type:    "unenforced_gate_setting",
				Message: "gate.max_error_recurrence_rate is configured while gate.enabled is false",
				Ref:     Ref(DefaultFile, gateLimit),
			})
		}
	case "true":
		gateLimit, ok := document.Scalars["gate.max_error_recurrence_rate"]
		if !ok {
			return nil, configErrorAt("gate.max_error_recurrence_rate is required when gate.enabled is true", Ref(DefaultFile, gateEnabled))
		}
		parsed, err := strconv.ParseFloat(gateLimit.Value, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || parsed > 1 {
			return nil, configErrorAt("gate.max_error_recurrence_rate must be a number between 0 and 1 when gate.enabled is true", Ref(DefaultFile, gateLimit))
		}
		warnings = append(warnings, Finding{
			Type:    "recurrence_gate_enabled",
			Message: "gate.enabled is true; relia backtest exits 5 when headline ERR exceeds the configured threshold",
			Ref:     Ref(DefaultFile, gateEnabled),
		})
	default:
		return nil, configErrorAt("gate.enabled must be true or false", Ref(DefaultFile, gateEnabled))
	}

	embeddings, ok := document.Scalars["distill.embeddings"]
	if !ok {
		return nil, configError("relia.yaml missing required key distill.embeddings")
	}
	switch embeddings.Value {
	case "signature", "local", "provider":
	default:
		return nil, configError("distill.embeddings must be signature, local, or provider")
	}
	effectiveEmbeddings := embeddings.Value
	effectiveEmbeddingsRef := Ref(DefaultFile, embeddings)
	if options.EmbeddingOverride != "" {
		effectiveEmbeddings = options.EmbeddingOverride
		effectiveEmbeddingsRef = "relia distill --embeddings"
	}
	switch effectiveEmbeddings {
	case "signature":
	case "local":
		manifest := document.Scalars["models.local_manifest"]
		if configErr := ValidateLocalModelManifest(root, manifest); configErr != nil {
			return nil, configErr
		}
	case "provider":
		if options.EmbeddingOverride == "" {
			return nil, dependencyError("provider embeddings require an approved model_provider_endpoint gate", effectiveEmbeddingsRef)
		}
		warnings = append(warnings, Finding{
			Type:    "provider_embedding_gate_required",
			Message: "provider embeddings are opt-in live model work and require a model_provider_endpoint gate before any network call",
			Ref:     effectiveEmbeddingsRef,
		})
	default:
		return nil, configError("distill.embeddings must be signature, local, or provider")
	}

	providerScalar, ok := document.Scalars["distill.provider"]
	if !ok {
		return nil, configError("relia.yaml missing required key distill.provider")
	}
	switch NormalizeDistillProvider(providerScalar.Value) {
	case "none":
	case "openai_compatible", "anthropic":
		providerConfig, configErr := ProviderConfigFromYAML(document)
		if configErr != nil {
			return nil, configErr
		}
		warnings = append(warnings, Finding{
			Type:    "provider_data_disclosure",
			Message: "provider-backed distill may send redacted experience records outside the machine only after an approved model_provider_endpoint gate; credential values are not read by relia check",
			Ref:     providerConfig.ProviderRef,
		})
	default:
		return nil, configError("distill.provider must be none, openai_compatible, openai-compatible, or anthropic")
	}

	reviewRequired, ok := document.Scalars["distill.review_required"]
	if !ok {
		return nil, configError("relia.yaml missing required key distill.review_required")
	}
	switch reviewRequired.Value {
	case "true":
	case "false":
		warnings = append(warnings, Finding{
			Type:    "review_gate_disabled",
			Message: "distill.review_required is disabled, but drafted rules still require explicit review before activation in the MVP",
			Ref:     Ref(DefaultFile, reviewRequired),
		})
	default:
		return nil, configError("distill.review_required must be true or false")
	}
	if len(yamlmini.ListValuesWithMapFields(document, "attribution.agent_authors", "login")) == 0 &&
		len(yamlmini.ListValues(document, "attribution.coauthor_trailers")) == 0 &&
		len(yamlmini.ListValues(document, "attribution.pr_labels")) == 0 {
		return nil, artifactContractError("attribution config has zero agent matchers; configure at least one agent_authors login, coauthor_trailer, or pr_label", PathRef(DefaultFile, document, "attribution"))
	}
	if _, configErr := AdviseSettingsFromConfig(document); configErr != nil {
		return nil, configErr
	}
	return warnings, nil
}

func DistillProvider(document yamlmini.Document) (string, string) {
	if scalar, ok := document.Scalars["distill.provider"]; ok {
		return NormalizeDistillProvider(scalar.Value), Ref(DefaultFile, scalar)
	}
	return "none", DefaultFile
}

func NormalizeDistillProvider(value string) string {
	switch strings.TrimSpace(value) {
	case "", "none":
		return "none"
	case "openai-compatible", "openai_compatible":
		return "openai_compatible"
	case "anthropic":
		return "anthropic"
	default:
		return strings.TrimSpace(value)
	}
}

func ProviderConfigFromYAML(document yamlmini.Document) (ProviderConfig, *Error) {
	provider, providerRef := DistillProvider(document)
	cfg := ProviderConfig{
		Provider:    provider,
		ProviderRef: providerRef,
	}
	if provider == "none" {
		return cfg, nil
	}
	if !ProviderSupported(provider) {
		return cfg, configErrorAt("distill.provider must be none, openai_compatible, openai-compatible, or anthropic", providerRef)
	}
	required := []struct {
		key    string
		target *string
	}{
		{key: "distill.model", target: &cfg.Model},
		{key: "distill.base_url", target: &cfg.BaseURL},
		{key: "distill.credential_env", target: &cfg.CredentialEnv},
	}
	for _, item := range required {
		scalar, ok := document.Scalars[item.key]
		if !ok || strings.TrimSpace(scalar.Value) == "" {
			return cfg, configErrorAt(item.key+" is required when distill.provider is "+provider, providerRef)
		}
		*item.target = strings.TrimSpace(scalar.Value)
	}
	if !validCredentialEnvName(cfg.CredentialEnv) {
		scalar := document.Scalars["distill.credential_env"]
		return cfg, configErrorAt("distill.credential_env must name an environment variable, not a secret value", Ref(DefaultFile, scalar))
	}
	if configErr := validateProviderBaseURL(cfg.BaseURL, document.Scalars["distill.base_url"]); configErr != nil {
		return cfg, configErr
	}
	var configErr *Error
	cfg.MaxCostUSDPerRun, configErr = requiredYAMLFloat(document, "distill.max_cost_usd_per_run", providerRef)
	if configErr != nil {
		return cfg, configErr
	}
	cfg.InputCostUSDPer1KTokens, configErr = requiredYAMLFloat(document, "distill.input_cost_usd_per_1k_tokens", providerRef)
	if configErr != nil {
		return cfg, configErr
	}
	cfg.OutputCostUSDPer1KTokens, configErr = requiredYAMLFloat(document, "distill.output_cost_usd_per_1k_tokens", providerRef)
	if configErr != nil {
		return cfg, configErr
	}
	for key, value := range map[string]float64{
		"distill.max_cost_usd_per_run":          cfg.MaxCostUSDPerRun,
		"distill.input_cost_usd_per_1k_tokens":  cfg.InputCostUSDPer1KTokens,
		"distill.output_cost_usd_per_1k_tokens": cfg.OutputCostUSDPer1KTokens,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return cfg, configErrorAt(key+" must be a non-negative number", Ref(DefaultFile, document.Scalars[key]))
		}
	}
	return cfg, nil
}

func ProviderSupported(provider string) bool {
	switch NormalizeDistillProvider(provider) {
	case "openai_compatible", "anthropic":
		return true
	default:
		return false
	}
}

func AdviseSettingsFromConfig(document yamlmini.Document) (AdviseSettings, *Error) {
	settings := AdviseSettings{
		Enabled:                 true,
		MaxCommentsPerPR:        1,
		UpdateInPlace:           true,
		ReassessDebounceMinutes: 10,
		MinConfidence:           0.6,
	}
	if scalar, ok := document.Scalars["advise.enabled"]; ok {
		switch scalar.Value {
		case "true":
			settings.Enabled = true
		case "false":
			settings.Enabled = false
		default:
			return settings, configErrorAt("advise.enabled must be true or false", Ref(DefaultFile, scalar))
		}
	}
	if scalar, ok := document.Scalars["advise.max_comments_per_pr"]; ok {
		parsed, err := strconv.Atoi(scalar.Value)
		if err != nil || parsed < 0 {
			return settings, configErrorAt("advise.max_comments_per_pr must be a non-negative integer", Ref(DefaultFile, scalar))
		}
		settings.MaxCommentsPerPR = parsed
	}
	if settings.MaxCommentsPerPR > 1 {
		return settings, configErrorAt("advise.max_comments_per_pr must be 0 or 1 for MVP advisory restraint", PathRef(DefaultFile, document, "advise.max_comments_per_pr"))
	}
	if scalar, ok := document.Scalars["advise.update_in_place"]; ok {
		switch scalar.Value {
		case "true":
			settings.UpdateInPlace = true
		case "false":
			settings.UpdateInPlace = false
		default:
			return settings, configErrorAt("advise.update_in_place must be true or false", Ref(DefaultFile, scalar))
		}
	}
	if !settings.UpdateInPlace && settings.MaxCommentsPerPR == 1 {
		return settings, configErrorAt("advise.update_in_place must remain true when advisory comments are enabled", PathRef(DefaultFile, document, "advise.update_in_place"))
	}
	if scalar, ok := document.Scalars["advise.reassess_debounce_minutes"]; ok {
		parsed, err := strconv.Atoi(scalar.Value)
		if err != nil || parsed < 0 {
			return settings, configErrorAt("advise.reassess_debounce_minutes must be a non-negative integer", Ref(DefaultFile, scalar))
		}
		settings.ReassessDebounceMinutes = parsed
	}
	if scalar, ok := document.Scalars["advise.min_confidence"]; ok {
		parsed, err := strconv.ParseFloat(scalar.Value, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || parsed > 1 {
			return settings, configErrorAt("advise.min_confidence must be a number between 0 and 1", Ref(DefaultFile, scalar))
		}
		settings.MinConfidence = parsed
	}
	return settings, nil
}

func ValidateLocalModelManifest(root string, manifestScalar yamlmini.Scalar) *Error {
	manifestRel := strings.TrimSpace(manifestScalar.Value)
	if manifestRel == "" || filepath.IsAbs(manifestRel) {
		return dependencyError("local model manifest path must be repo-relative", Ref(DefaultFile, manifestScalar))
	}
	manifestPath := filepath.Join(root, manifestRel)
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return dependencyError("local embedding artifact manifest is missing", Ref(DefaultFile, manifestScalar))
		}
		return internalError("could not read local model manifest", err)
	}
	var manifest LocalModelManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return dependencyError("local model manifest is not valid JSON", manifestRel)
	}
	return ValidateLocalModelManifestPayload(root, manifest, manifestRel)
}

func ValidateLocalModelManifestPayload(root string, manifest LocalModelManifest, ref string) *Error {
	required := map[string]string{
		"model_id":        manifest.ModelID,
		"version":         manifest.Version,
		"source_url":      manifest.SourceURL,
		"license":         manifest.License,
		"digest":          manifest.Digest,
		"cache_path":      manifest.CachePath,
		"update_policy":   manifest.UpdatePolicy,
		"rollback_policy": manifest.RollbackPolicy,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return dependencyError("local model manifest missing required field "+field, ref)
		}
	}
	if !strings.HasPrefix(manifest.SourceURL, "https://") {
		return dependencyError("local model manifest source_url must be https", ref)
	}
	digest := CanonicalModelDigest(manifest.Digest)
	if len(digest) != 64 || !isHexDigest(digest) {
		return dependencyError("local model manifest digest must be a SHA-256 hex digest", ref)
	}
	switch manifest.Status {
	case "", "ready":
	case "stale":
		return dependencyError("local model artifact is stale", ref)
	default:
		return dependencyError("local model manifest status must be ready or stale", ref)
	}
	cachePath := filepath.Clean(manifest.CachePath)
	cachePathSlash := filepath.ToSlash(cachePath)
	if filepath.IsAbs(manifest.CachePath) || cachePath == "." || cachePath == ".." || strings.HasPrefix(cachePathSlash, "../") {
		return dependencyError("local model manifest cache_path must stay inside the repository", ref)
	}
	if cachePathSlash == ".relia/models" || !strings.HasPrefix(cachePathSlash, ".relia/models/") {
		return dependencyError("local model manifest cache_path must stay under .relia/models", ref)
	}
	artifactPath := filepath.Join(root, cachePath)
	artifactContent, err := os.ReadFile(artifactPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return dependencyError("local model artifact is missing", ref)
		}
		return internalError("could not read local model artifact", err)
	}
	actual := fmt.Sprintf("%x", sha256.Sum256(artifactContent))
	if actual != digest {
		return dependencyError("local model artifact digest does not match manifest", ref)
	}
	return nil
}

func CanonicalModelDigest(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
}

func requiredYAMLFloat(document yamlmini.Document, key string, fallbackRef string) (float64, *Error) {
	scalar, ok := document.Scalars[key]
	if !ok || strings.TrimSpace(scalar.Value) == "" {
		return 0, configErrorAt(key+" is required when provider-backed distill is configured", fallbackRef)
	}
	parsed, err := strconv.ParseFloat(scalar.Value, 64)
	if err != nil {
		return 0, configErrorAt(key+" must be a number", Ref(DefaultFile, scalar))
	}
	return parsed, nil
}

func validCredentialEnvName(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && index > 0:
		case r == '_' && index > 0:
		default:
			return false
		}
	}
	return true
}

func validateProviderBaseURL(value string, scalar yamlmini.Scalar) *Error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return configErrorAt("distill.base_url must be an https URL for provider-backed distill", Ref(DefaultFile, scalar))
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return configErrorAt("distill.base_url must not include query or fragment components", Ref(DefaultFile, scalar))
	}
	if parsed.User != nil {
		return configErrorAt("distill.base_url must not include user info or embedded credentials", Ref(DefaultFile, scalar))
	}
	return nil
}

func isHexDigest(value string) bool {
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func configError(message string) *Error {
	return &Error{Kind: ErrorConfig, Message: message, Ref: DefaultFile}
}

func configErrorAt(message string, ref string) *Error {
	return &Error{Kind: ErrorConfig, Message: message, Ref: ref}
}

func artifactContractError(message string, ref string) *Error {
	return &Error{Kind: ErrorArtifactContract, Message: message, Ref: ref}
}

func redactionSafetyError(message string, ref string) *Error {
	return &Error{Kind: ErrorRedactionSafety, Message: message, Ref: ref}
}

func dependencyError(message string, ref string) *Error {
	return &Error{Kind: ErrorDependency, Message: message, Ref: ref}
}

func internalError(message string, err error) *Error {
	return &Error{Kind: ErrorInternal, Message: message, Err: err}
}
