package config

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

func FindRepoRoot(start string) (string, bool) {
	current := filepath.Clean(start)
	for {
		goMod := filepath.Join(current, "go.mod")
		content, err := os.ReadFile(goMod)
		if err == nil && strings.Contains(string(content), "module github.com/Clyra-AI/relia") {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func RepoPathExists(root string, rel string) bool {
	clean, ok := CleanRepoPath(rel)
	if !ok {
		return false
	}
	if WorkingTreePathMatches(root, clean) {
		return true
	}
	if output, err := exec.Command("git", "-C", root, "log", "--all", "--name-only", "--format=", "--", clean).Output(); err == nil {
		return strings.TrimSpace(string(output)) != ""
	}
	return false
}

func CleanRepoPath(rel string) (string, bool) {
	trimmed := strings.TrimSpace(rel)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return "", false
	}
	clean := filepath.Clean(trimmed)
	cleanSlash := filepath.ToSlash(clean)
	if clean == "." || clean == ".." || strings.HasPrefix(cleanSlash, "../") {
		return "", false
	}
	for _, part := range strings.Split(cleanSlash, "/") {
		if part == ".." {
			return "", false
		}
	}
	return clean, true
}

func WorkingTreePathMatches(root string, scope string) bool {
	scopeSlash := filepath.ToSlash(scope)
	if !HasGlobMagic(scopeSlash) {
		_, err := os.Stat(filepath.Join(root, scope))
		return err == nil
	}
	matched := false
	_ = filepath.WalkDir(root, func(candidate string, entry os.DirEntry, err error) error {
		if matched {
			return filepath.SkipAll
		}
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".factory", ".factoryd", ".relia":
				if candidate != root {
					return filepath.SkipDir
				}
			}
		}
		rel, err := filepath.Rel(root, candidate)
		if err != nil || rel == "." {
			return nil
		}
		if ScopePatternMatches(scopeSlash, filepath.ToSlash(rel)) {
			matched = true
			return filepath.SkipAll
		}
		return nil
	})
	return matched
}

func HasGlobMagic(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

func ScopePatternMatches(pattern string, rel string) bool {
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return rel == prefix || strings.HasPrefix(rel, prefix+"/")
	}
	matched, err := path.Match(pattern, rel)
	return err == nil && matched
}
