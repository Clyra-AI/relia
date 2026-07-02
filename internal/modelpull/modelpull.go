package modelpull

import (
	"fmt"
	"path/filepath"
	"strings"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	"github.com/Clyra-AI/relia/internal/yamlmini"
)

type Options struct {
	ModelID        string
	Version        string
	SourceURL      string
	License        string
	Digest         string
	CachePath      string
	UpdatePolicy   string
	RollbackPolicy string
}

type ParseError struct {
	Message string
}

func (e ParseError) Error() string {
	return e.Message
}

type PathErrorKind string

const (
	PathErrorUsage      PathErrorKind = "usage"
	PathErrorDependency PathErrorKind = "dependency"
)

type PathError struct {
	Kind      PathErrorKind
	Message   string
	Reference string
}

func (e *PathError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func ParseArgs(args []string) (Options, *ParseError) {
	var options Options
	for index := 0; index < len(args); index++ {
		arg := args[index]
		needValue := func(message string) (string, bool) {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return message, false
			}
			return args[index+1], true
		}
		switch arg {
		case "--model-id":
			value, ok := needValue("models pull requires a value after --model-id")
			if !ok {
				return options, &ParseError{Message: value}
			}
			options.ModelID = value
			index++
		case "--version":
			value, ok := needValue("models pull requires a value after --version")
			if !ok {
				return options, &ParseError{Message: value}
			}
			options.Version = value
			index++
		case "--source-url":
			value, ok := needValue("models pull requires a value after --source-url")
			if !ok {
				return options, &ParseError{Message: value}
			}
			options.SourceURL = value
			index++
		case "--license":
			value, ok := needValue("models pull requires a value after --license")
			if !ok {
				return options, &ParseError{Message: value}
			}
			options.License = value
			index++
		case "--digest":
			value, ok := needValue("models pull requires a value after --digest")
			if !ok {
				return options, &ParseError{Message: value}
			}
			options.Digest = value
			index++
		case "--cache-path":
			value, ok := needValue("models pull requires a repo-relative path after --cache-path")
			if !ok {
				return options, &ParseError{Message: value}
			}
			options.CachePath = value
			index++
		case "--update-policy":
			value, ok := needValue("models pull requires a value after --update-policy")
			if !ok {
				return options, &ParseError{Message: value}
			}
			options.UpdatePolicy = value
			index++
		case "--rollback-policy":
			value, ok := needValue("models pull requires a value after --rollback-policy")
			if !ok {
				return options, &ParseError{Message: value}
			}
			options.RollbackPolicy = value
			index++
		default:
			return options, &ParseError{Message: fmt.Sprintf("unknown models pull argument %q", arg)}
		}
	}
	for _, item := range []struct {
		flag  string
		value string
	}{
		{"--model-id", options.ModelID},
		{"--version", options.Version},
		{"--source-url", options.SourceURL},
		{"--license", options.License},
		{"--digest", options.Digest},
		{"--cache-path", options.CachePath},
		{"--update-policy", options.UpdatePolicy},
		{"--rollback-policy", options.RollbackPolicy},
	} {
		if strings.TrimSpace(item.value) == "" {
			return options, &ParseError{Message: "models pull requires " + item.flag}
		}
	}
	return options, nil
}

func Manifest(options Options, cachePath string) configdoc.LocalModelManifest {
	return configdoc.LocalModelManifest{
		ModelID:        options.ModelID,
		Version:        options.Version,
		SourceURL:      options.SourceURL,
		License:        options.License,
		Digest:         configdoc.CanonicalModelDigest(options.Digest),
		CachePath:      cachePath,
		UpdatePolicy:   options.UpdatePolicy,
		RollbackPolicy: options.RollbackPolicy,
		Status:         "ready",
	}
}

func ConfiguredManifestPath(scalars map[string]yamlmini.Scalar) string {
	if scalar, ok := scalars["models.local_manifest"]; ok {
		return scalar.Value
	}
	return ".relia/models/manifest.json"
}

func PrepareManifest(options Options, manifestRel string, configRef string) (configdoc.LocalModelManifest, string, *PathError) {
	cleanManifestRel, ok := configdoc.CleanRepoPath(manifestRel)
	if !ok {
		return configdoc.LocalModelManifest{}, "", &PathError{
			Kind:      PathErrorDependency,
			Message:   "local model manifest path must be repo-relative",
			Reference: configRef,
		}
	}
	manifestDisplayPath := filepath.ToSlash(filepath.Clean(cleanManifestRel))
	cleanCachePath, ok := configdoc.CleanRepoPath(options.CachePath)
	if !ok {
		return configdoc.LocalModelManifest{}, "", &PathError{
			Kind:    PathErrorUsage,
			Message: "models pull --cache-path must be repo-relative",
		}
	}
	cachePath := filepath.ToSlash(filepath.Clean(cleanCachePath))
	if cachePath == manifestDisplayPath {
		return configdoc.LocalModelManifest{}, "", &PathError{
			Kind:    PathErrorUsage,
			Message: "models pull --cache-path must not equal the local model manifest path",
		}
	}
	return Manifest(options, cachePath), manifestDisplayPath, nil
}

func ResultData(manifest configdoc.LocalModelManifest, manifestDisplayPath string) map[string]any {
	return map[string]any{
		"model_id":        manifest.ModelID,
		"version":         manifest.Version,
		"source_url":      manifest.SourceURL,
		"license":         manifest.License,
		"digest":          manifest.Digest,
		"cache_path":      manifest.CachePath,
		"update_policy":   manifest.UpdatePolicy,
		"rollback_policy": manifest.RollbackPolicy,
		"manifest_path":   manifestDisplayPath,
		"network_used":    false,
		"status":          manifest.Status,
	}
}
