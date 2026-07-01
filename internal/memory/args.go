package memory

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Options struct {
	Format     string
	OutputPath string
}

type ParseError struct {
	Message string
}

func (e ParseError) Error() string {
	return e.Message
}

func ParseArgs(args []string) (Options, *ParseError) {
	options := Options{Format: "json", OutputPath: "memory/MEMORY.md"}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--format":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "memory requires a value after --format"}
			}
			options.Format = args[index+1]
			index++
		case "--output":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "memory requires a repo-relative path after --output"}
			}
			options.OutputPath = args[index+1]
			index++
		default:
			return options, &ParseError{Message: fmt.Sprintf("unknown memory argument %q", arg)}
		}
	}
	if options.Format != "json" {
		return options, &ParseError{Message: "memory only supports --format json in this task slice"}
	}
	if _, ok := cleanRepoPath(options.OutputPath); !ok {
		return options, &ParseError{Message: "memory --output must be a repo-relative path"}
	}
	return options, nil
}

func cleanRepoPath(rel string) (string, bool) {
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
