package distill

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type Options struct {
	Format       string
	InputPath    string
	RuleDir      string
	HalfLifeDays int
	Embeddings   string
}

type ParseError struct {
	Message string
}

func (e ParseError) Error() string {
	return e.Message
}

func ParseArgs(args []string) (Options, *ParseError) {
	options := Options{
		Format:       "json",
		RuleDir:      "memory/rules",
		HalfLifeDays: 90,
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--format":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "distill requires a value after --format"}
			}
			options.Format = args[index+1]
			index++
		case "--input", "-i":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "distill requires a path after --input"}
			}
			if strings.TrimSpace(args[index+1]) == "" {
				return options, &ParseError{Message: "distill --input must be a non-empty path"}
			}
			options.InputPath = args[index+1]
			index++
		case "--rule-dir":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "distill requires a repo-relative path after --rule-dir"}
			}
			options.RuleDir = args[index+1]
			index++
		case "--half-life-days":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "distill requires a positive integer after --half-life-days"}
			}
			parsed, err := strconv.Atoi(args[index+1])
			if err != nil || parsed <= 0 {
				return options, &ParseError{Message: "distill --half-life-days must be a positive integer"}
			}
			options.HalfLifeDays = parsed
			index++
		case "--embeddings":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "distill requires signature, local, or provider after --embeddings"}
			}
			options.Embeddings = args[index+1]
			index++
		default:
			return options, &ParseError{Message: fmt.Sprintf("unknown distill argument %q", arg)}
		}
	}
	if options.Format != "json" {
		return options, &ParseError{Message: "distill only supports --format json in this task slice"}
	}
	if _, ok := cleanRepoPath(options.RuleDir); !ok {
		return options, &ParseError{Message: "distill --rule-dir must be a repo-relative path"}
	}
	switch options.Embeddings {
	case "", "signature", "local", "provider":
	default:
		return options, &ParseError{Message: "distill --embeddings must be signature, local, or provider"}
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
