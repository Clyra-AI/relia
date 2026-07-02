package config

import (
	"os"
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
