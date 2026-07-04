package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
	resultdoc "github.com/Clyra-AI/relia/internal/result"
	"github.com/Clyra-AI/relia/internal/yamlmini"
)

type ValidationOptions struct {
	SchemaVersion         string
	ArtifactContractError func(string, string) *resultdoc.CommandError
	InternalError         func(string, error) *resultdoc.CommandError
	RepoPathExists        func(string, string) bool
	YAMLFloat             func(float64) string
}

func ValidateRuleArtifacts(root string, options ValidationOptions) *resultdoc.CommandError {
	patterns := []string{
		filepath.Join(root, "memory", "rules", "*.yaml"),
		filepath.Join(root, "memory", "rules", "*.yml"),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return internalError(options, "could not inspect memory rule artifacts", err)
		}
		for _, path := range matches {
			if commandErr := ValidateRuleArtifact(root, path, options); commandErr != nil {
				return commandErr
			}
		}
	}
	return nil
}

func ValidateRuleArtifact(root string, path string, options ValidationOptions) *resultdoc.CommandError {
	content, err := os.ReadFile(path)
	if err != nil {
		return internalError(options, "could not read memory rule artifact", err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	document, parseErr := yamlmini.ParseDocument(string(content))
	if parseErr != nil {
		return artifactContractError(options, parseErr.Error(), rel)
	}
	required := []string{"object_type", "schema_version", "id", "kind", "status", "statement", "scope", "confidence", "evidence", "provenance", "review", "metadata"}
	for _, key := range required {
		if !yamlmini.HasPath(document, key) {
			return artifactContractError(options, "memory rule missing required key "+key, rel)
		}
	}
	if document.Scalars["object_type"].Value != "relia.memory_rule" {
		return artifactContractError(options, "memory rule object_type must be relia.memory_rule", configdoc.RefWithPath(rel, document.Scalars["object_type"]))
	}
	if document.Scalars["schema_version"].Value != options.SchemaVersion {
		return artifactContractError(options, "memory rule schema_version must be "+options.SchemaVersion, rel)
	}
	kind := document.Scalars["kind"].Value
	if kind != "avoid" && kind != "playbook" {
		return artifactContractError(options, "memory rule kind must be avoid or playbook", configdoc.RefWithPath(rel, document.Scalars["kind"]))
	}
	status := document.Scalars["status"].Value
	switch status {
	case "candidate", "active", "stale", "contradicted", "retired":
	default:
		return artifactContractError(options, "memory rule status is invalid", configdoc.RefWithPath(rel, document.Scalars["status"]))
	}
	confidence, err := strconv.ParseFloat(document.Scalars["confidence"].Value, 64)
	if err != nil || confidence < 0 || confidence > 1 {
		return artifactContractError(options, "memory rule confidence must be between 0 and 1", configdoc.RefWithPath(rel, document.Scalars["confidence"]))
	}
	if len(document.Lists["evidence.experiences"]) == 0 {
		return artifactContractError(options, "memory rule must cite at least one experience", rel)
	}
	if len(document.Lists["scope.paths"]) == 0 && len(document.Lists["scope.signals"]) == 0 {
		return artifactContractError(options, "memory rule must declare at least one scope path or signal", rel)
	}
	for _, scopePath := range document.Lists["scope.paths"] {
		if status != "stale" && !repoPathExists(options, root, scopePath.Value) {
			return artifactContractError(options, "memory rule scope path does not exist in the repo", configdoc.RefWithPath(rel, scopePath))
		}
	}
	evidenceCount, ok := document.Scalars["evidence.count"]
	if !ok {
		return artifactContractError(options, "memory rule missing required key evidence.count", rel)
	}
	count, err := strconv.Atoi(evidenceCount.Value)
	if err != nil || count < 1 {
		return artifactContractError(options, "memory rule evidence.count must be at least 1", configdoc.RefWithPath(rel, evidenceCount))
	}
	contradictionsScalar, ok := document.Scalars["evidence.contradictions"]
	if !ok {
		return artifactContractError(options, "memory rule missing required key evidence.contradictions", rel)
	}
	contradictions, err := strconv.Atoi(contradictionsScalar.Value)
	if err != nil || contradictions < 0 {
		return artifactContractError(options, "memory rule evidence.contradictions must be at least 0", configdoc.RefWithPath(rel, contradictionsScalar))
	}
	provenanceEntries := document.Lists["provenance"]
	if len(provenanceEntries) == 0 {
		return artifactContractError(options, "memory rule must include at least one provenance entry", rel)
	}
	provenanceMaps := document.ListMaps["provenance"]
	if len(provenanceMaps) != len(provenanceEntries) {
		return artifactContractError(options, "memory rule provenance entries must include pr and outcome", rel)
	}
	hasPositivePlaybookEvidence := false
	for _, provenance := range provenanceMaps {
		pr, ok := provenance["pr"]
		if !ok {
			return artifactContractError(options, "memory rule provenance entry missing pr", rel)
		}
		prNumber, err := strconv.Atoi(pr.Value)
		if err != nil || prNumber < 1 {
			return artifactContractError(options, "memory rule provenance pr must be at least 1", configdoc.RefWithPath(rel, pr))
		}
		provenanceURL, ok := provenance["url"]
		if !ok || strings.TrimSpace(provenanceURL.Value) == "" {
			return artifactContractError(options, "memory rule provenance url is required", rel)
		}
		provenanceURLPR, ok := ingestdoc.GitHubPullRequestURLNumber(provenanceURL.Value)
		if !ok {
			return artifactContractError(options, "memory rule provenance url must be an https://github.com/<owner>/<repo>/pull/<number> URL", configdoc.RefWithPath(rel, provenanceURL))
		}
		if provenanceURLPR != prNumber {
			return artifactContractError(options, "memory rule provenance url pull number must match provenance pr", configdoc.RefWithPath(rel, provenanceURL))
		}
		outcome, ok := provenance["outcome"]
		if !ok {
			return artifactContractError(options, "memory rule provenance entry missing outcome", rel)
		}
		switch outcome.Value {
		case "ci_failure", "revert", "review_correction", "fix_held", "merged_clean":
			if outcome.Value == "fix_held" || outcome.Value == "merged_clean" {
				hasPositivePlaybookEvidence = true
			}
		default:
			return artifactContractError(options, "memory rule provenance outcome is invalid", configdoc.RefWithPath(rel, outcome))
		}
	}
	if kind == "playbook" && !hasPositivePlaybookEvidence {
		return artifactContractError(options, "playbook memory rule must cite at least one fix_held or merged_clean provenance outcome", rel)
	}
	reviewLabel, ok := document.Scalars["review.label"]
	if !ok {
		return artifactContractError(options, "memory rule missing required key review.label", rel)
	}
	switch reviewLabel.Value {
	case "accepted", "suggested", "needs_user_input":
	default:
		return artifactContractError(options, "memory rule review.label is invalid", configdoc.RefWithPath(rel, reviewLabel))
	}
	if status == "active" && reviewLabel.Value != "accepted" {
		return artifactContractError(options, "active memory rule review.label must be accepted", configdoc.RefWithPath(rel, reviewLabel))
	}
	if status != "active" && reviewLabel.Value == "accepted" {
		return artifactContractError(options, "accepted memory rule status must be active", configdoc.RefWithPath(rel, reviewLabel))
	}
	if reviewGate, ok := document.Scalars["review.gate"]; ok {
		switch reviewGate.Value {
		case "human_review":
		default:
			return artifactContractError(options, "memory rule review.gate is invalid", configdoc.RefWithPath(rel, reviewGate))
		}
	}
	if reviewDecision, ok := document.Scalars["review.decision"]; ok {
		switch reviewDecision.Value {
		case "pending", "approved", "needs_user_input", "rejected", "merged":
		default:
			return artifactContractError(options, "memory rule review.decision is invalid", configdoc.RefWithPath(rel, reviewDecision))
		}
		if status == "active" && reviewDecision.Value != "approved" {
			return artifactContractError(options, "active memory rule review.decision must be approved", configdoc.RefWithPath(rel, reviewDecision))
		}
		if reviewDecision.Value == "approved" && status != "active" {
			return artifactContractError(options, "approved memory rule status must be active", configdoc.RefWithPath(rel, reviewDecision))
		}
		if (reviewDecision.Value == "rejected" || reviewDecision.Value == "merged") && status != "retired" {
			return artifactContractError(options, "rejected or merged memory rule status must be retired", configdoc.RefWithPath(rel, reviewDecision))
		}
		if reviewDecision.Value == "merged" {
			mergedInto, ok := document.Scalars["review.merged_into"]
			if !ok || strings.TrimSpace(mergedInto.Value) == "" {
				return artifactContractError(options, "merged memory rule review.merged_into is required", configdoc.RefWithPath(rel, reviewDecision))
			}
		}
	}
	statementOrigin, ok := document.Scalars["review.statement_origin"]
	if !ok {
		return artifactContractError(options, "memory rule missing required key review.statement_origin", rel)
	}
	switch statementOrigin.Value {
	case "llm_drafted", "cluster_summary", "human_authored":
	default:
		return artifactContractError(options, "memory rule review.statement_origin is invalid", configdoc.RefWithPath(rel, statementOrigin))
	}
	if commandErr := ValidateDraftedRuleCalibration(document, rel, confidence, count, contradictions, options); commandErr != nil {
		return commandErr
	}
	return nil
}

func ValidateDraftedRuleCalibration(document yamlmini.Document, rel string, confidence float64, evidenceCount int, contradictions int, options ValidationOptions) *resultdoc.CommandError {
	statementOrigin := document.Scalars["review.statement_origin"].Value
	if statementOrigin == "human_authored" {
		return nil
	}
	confidenceLabel, ok := document.Scalars["metadata.confidence_label"]
	if !ok {
		return artifactContractError(options, "drafted memory rule missing required key metadata.confidence_label", rel)
	}
	if confidenceLabel.Value != confidenceLabelForRule(confidence) {
		return artifactContractError(options, "drafted memory rule metadata.confidence_label must match confidence", configdoc.RefWithPath(rel, confidenceLabel))
	}

	inputCount, commandErr := requiredYAMLIntAtLeast(document, rel, "metadata.confidence_inputs.evidence_count", 1, options)
	if commandErr != nil {
		return commandErr
	}
	if inputCount != evidenceCount {
		return artifactContractError(options, "drafted memory rule metadata.confidence_inputs.evidence_count must match evidence.count", configdoc.RefWithPath(rel, document.Scalars["metadata.confidence_inputs.evidence_count"]))
	}
	inputContradictions, commandErr := requiredYAMLIntAtLeast(document, rel, "metadata.confidence_inputs.contradictions", 0, options)
	if commandErr != nil {
		return commandErr
	}
	if inputContradictions != contradictions {
		return artifactContractError(options, "drafted memory rule metadata.confidence_inputs.contradictions must match evidence.contradictions", configdoc.RefWithPath(rel, document.Scalars["metadata.confidence_inputs.contradictions"]))
	}
	for _, key := range []string{
		"metadata.confidence_inputs.recency_weight",
		"metadata.confidence_inputs.flake_discount",
		"metadata.confidence_inputs.extraction_confidence",
	} {
		if _, commandErr := requiredYAMLFloatRange(document, rel, key, 0, 1, options); commandErr != nil {
			return commandErr
		}
	}
	draftingModelWeight, commandErr := requiredYAMLFloatRange(document, rel, "metadata.confidence_inputs.drafting_model_weight", 0, 0, options)
	if commandErr != nil {
		return commandErr
	}
	if draftingModelWeight != 0 {
		return artifactContractError(options, "drafted memory rule metadata.confidence_inputs.drafting_model_weight must be 0", configdoc.RefWithPath(rel, document.Scalars["metadata.confidence_inputs.drafting_model_weight"]))
	}
	if _, commandErr := requiredYAMLIntAtLeast(document, rel, "metadata.decay.half_life_days", 1, options); commandErr != nil {
		return commandErr
	}
	for _, key := range []string{
		"metadata.decay.latest_evidence_at",
		"metadata.decay.oldest_evidence_at",
		"metadata.decay.anchor_recorded_at",
	} {
		if commandErr := requiredYAMLRFC3339(document, rel, key, options); commandErr != nil {
			return commandErr
		}
	}
	return nil
}

func confidenceLabelForRule(confidence float64) string {
	switch {
	case confidence >= 0.75:
		return "high"
	case confidence >= 0.5:
		return "medium"
	default:
		return "low"
	}
}

func requiredYAMLIntAtLeast(document yamlmini.Document, rel string, key string, minimum int, options ValidationOptions) (int, *resultdoc.CommandError) {
	scalar, ok := document.Scalars[key]
	if !ok {
		return 0, artifactContractError(options, "drafted memory rule missing required key "+key, rel)
	}
	value, err := strconv.Atoi(scalar.Value)
	if err != nil || value < minimum {
		return 0, artifactContractError(options, "drafted memory rule "+key+" must be at least "+strconv.Itoa(minimum), configdoc.RefWithPath(rel, scalar))
	}
	return value, nil
}

func requiredYAMLFloatRange(document yamlmini.Document, rel string, key string, minimum float64, maximum float64, options ValidationOptions) (float64, *resultdoc.CommandError) {
	scalar, ok := document.Scalars[key]
	if !ok {
		return 0, artifactContractError(options, "drafted memory rule missing required key "+key, rel)
	}
	value, err := strconv.ParseFloat(scalar.Value, 64)
	if err != nil || value < minimum || value > maximum {
		return 0, artifactContractError(options, "drafted memory rule "+key+" must be between "+yamlFloat(options, minimum)+" and "+yamlFloat(options, maximum), configdoc.RefWithPath(rel, scalar))
	}
	return value, nil
}

func requiredYAMLRFC3339(document yamlmini.Document, rel string, key string, options ValidationOptions) *resultdoc.CommandError {
	scalar, ok := document.Scalars[key]
	if !ok {
		return artifactContractError(options, "drafted memory rule missing required key "+key, rel)
	}
	if _, err := time.Parse(time.RFC3339, scalar.Value); err != nil {
		return artifactContractError(options, "drafted memory rule "+key+" must be RFC3339", configdoc.RefWithPath(rel, scalar))
	}
	return nil
}

func repoPathExists(options ValidationOptions, root string, rel string) bool {
	if options.RepoPathExists != nil {
		return options.RepoPathExists(root, rel)
	}
	_, err := os.Stat(filepath.Join(root, rel))
	return err == nil
}

func artifactContractError(options ValidationOptions, message string, ref string) *resultdoc.CommandError {
	if options.ArtifactContractError != nil {
		return options.ArtifactContractError(message, ref)
	}
	return &resultdoc.CommandError{Type: "artifact_contract_validation_failed", Message: message, Ref: ref}
}

func internalError(options ValidationOptions, message string, err error) *resultdoc.CommandError {
	if options.InternalError != nil {
		return options.InternalError(message, err)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		message += ": " + err.Error()
	}
	return &resultdoc.CommandError{Type: "internal", Message: message}
}

func yamlFloat(options ValidationOptions, value float64) string {
	if options.YAMLFloat != nil {
		return options.YAMLFloat(value)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}
