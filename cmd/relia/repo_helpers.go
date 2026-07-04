package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
	memorydoc "github.com/Clyra-AI/relia/internal/memory"
	"github.com/Clyra-AI/relia/internal/yamlmini"
)

func readReliaConfig(root string) (yamlDocument, *CommandError) {
	document, configErr := configdoc.Read(root)
	return document, commandErrorFromConfig(configErr)
}

func resolveInputPath(root string, input string) string {
	if filepath.IsAbs(input) {
		return filepath.Clean(input)
	}
	return filepath.Clean(filepath.Join(root, input))
}

func displayPath(root string, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Join("external-input", filepath.Base(path)))
}

func writeRepoRelativeFile(root string, rel string, content []byte, label string) *CommandError {
	clean, ok := configdoc.CleanRepoPath(rel)
	if !ok {
		return usageError(label + " path must be repo-relative")
	}
	return writeAtomicRepoFile(filepath.Join(root, filepath.FromSlash(filepath.ToSlash(clean))), content, label)
}

func writeAtomicRepoFile(path string, content []byte, label string) *CommandError {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return internalError("could not create "+label+" directory", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return internalError("could not create temporary "+label, err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return internalError("could not write temporary "+label, err)
	}
	if err := tempFile.Close(); err != nil {
		return internalError("could not close temporary "+label, err)
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return internalError("could not set temporary "+label+" permissions", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return internalError("could not write "+label, err)
	}
	cleanup = false
	return nil
}

func writeMemoryPage(root string, outputPath string, rules []memorydoc.RuleSummary) (string, *CommandError) {
	clean, ok := configdoc.CleanRepoPath(outputPath)
	if !ok {
		return "", usageError("memory output path must be repo-relative")
	}
	rel := filepath.ToSlash(clean)
	path := filepath.Join(root, filepath.FromSlash(rel))
	if commandErr := writeAtomicRepoFile(path, []byte(memorydoc.RenderMarkdown(rules)), "memory page"); commandErr != nil {
		return "", commandErr
	}
	return rel, nil
}

func parseIngestEvents(content []byte, ref string) ([]map[string]any, *CommandError) {
	events, ingestErr := ingestdoc.ParseEvents(content, ref)
	return events, commandErrorFromIngest(ingestErr)
}

func parseIngestEventsForOptions(content []byte, ref string, options ingestOptions) ([]map[string]any, *CommandError) {
	if options.GitHubOutcomes {
		events, ingestErr := ingestdoc.ParseGitHubOutcomeEvents(content, ref)
		return events, commandErrorFromIngest(ingestErr)
	}
	return parseIngestEvents(content, ref)
}

func ingestSourceFormat(options ingestOptions) string {
	if options.GitHubOutcomes {
		return "github_outcomes"
	}
	return "outcome_events"
}

func decodeJSONUseNumber(input string, target any) error {
	return ingestdoc.DecodeJSONUseNumber(input, target)
}

func normalizeExperienceRecord(config yamlDocument, event map[string]any, index int, ref string) (experienceRecord, bool, *CommandError) {
	record, skipped, ingestErr := ingestdoc.NormalizeRecord(event, ingestdoc.RecordOptions{
		SchemaVersion:     commandSchemaVersion,
		AttributionPolicy: attributionPolicy(config),
		SourceIndex:       index,
	}, ref)
	return record, skipped, commandErrorFromIngest(ingestErr)
}

func attributionPolicy(document yamlDocument) ingestdoc.AttributionPolicy {
	policy := ingestdoc.AttributionPolicy{
		PRLabels:          yamlmini.ListValues(document, "attribution.pr_labels"),
		CoauthorTrailers:  yamlmini.ListValues(document, "attribution.coauthor_trailers"),
		AgentAuthorLogins: yamlmini.ListValuesWithMapFields(document, "attribution.agent_authors", "login"),
		Uncertain:         "exclude",
	}
	if scalar, ok := document.Scalars["attribution.uncertain"]; ok {
		switch scalar.Value {
		case "include_flagged":
			policy.Uncertain = "include_flagged"
		case "exclude":
			policy.Uncertain = "exclude"
		}
	}
	return policy
}

func persistExperienceRecords(root string, records []experienceRecord) ([]string, *CommandError) {
	shards, ingestErr := ingestdoc.PersistRecords(root, records)
	return shards, commandErrorFromIngest(ingestErr)
}

func redactForPersistence(event map[string]any, ref string) (any, *CommandError) {
	redacted, ingestErr := ingestdoc.RedactForPersistence(event, ref)
	return redacted, commandErrorFromIngest(ingestErr)
}

func gitHubProvenanceURLRepoMatchesExperience(value string, record experienceRecord) bool {
	return ingestdoc.GitHubProvenanceURLRepoMatchesRecord(value, record)
}

func gitHubPullRequestURLPathNumber(value string) (int, bool) {
	return ingestdoc.GitHubPullRequestURLPathNumber(value)
}

func gitHubPullRequestURLNumber(value string) (int, bool) {
	return ingestdoc.GitHubPullRequestURLNumber(value)
}

func gitHubPullRequestURLMatchesExperience(value string, record experienceRecord) bool {
	return ingestdoc.GitHubPullRequestURLMatchesRecord(value, record)
}

func gitHubPullRequestURLForExperience(record experienceRecord) string {
	return ingestdoc.GitHubPullRequestURLForRecord(record)
}

func sha256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + fmt.Sprintf("%x", digest)
}

func shortHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest)[:12]
}

func uniqueInts(values []int) []int {
	seen := map[int]struct{}{}
	var result []int
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
