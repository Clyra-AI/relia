package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var artifactSkeletonDirs = []string{
	".relia/experiences",
	".relia/signatures",
	".relia/coverage",
	".relia/reports",
	".relia/baselines",
	"memory/rules",
	"memory/compiled",
}

func ArtifactSkeletonPaths() []string {
	return append([]string(nil), artifactSkeletonDirs...)
}

func EnsureArtifactSkeleton(root string) error {
	for _, dir := range artifactSkeletonDirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func EnsureReliaGitIgnore(root string) error {
	path := filepath.Join(root, ".gitignore")
	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if reliaGitIgnoreContains(content) {
		return nil
	}
	next := strings.TrimRight(string(content), "\n")
	if next != "" {
		next += "\n"
	}
	next += ".relia/\n"
	return os.WriteFile(path, []byte(next), 0o644)
}

func reliaGitIgnoreContains(content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		switch strings.TrimSpace(line) {
		case ".relia", ".relia/", ".relia/*":
			return true
		}
	}
	return false
}
