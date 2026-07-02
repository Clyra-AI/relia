package advise

import (
	"fmt"
	"strings"

	configdoc "github.com/Clyra-AI/relia/internal/config"
)

type Options struct {
	InputPath   string
	Format      string
	StatePath   string
	CommentPath string
}

type ParseError struct {
	Message string
}

func (e ParseError) Error() string {
	return e.Message
}

func ParseArgs(args []string) (Options, *ParseError) {
	options := Options{
		Format:      "json",
		StatePath:   ".relia/reports/advisory-state.json",
		CommentPath: ".relia/reports/advisory-comment.md",
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--input", "--diff", "-i":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "advise requires a path after " + arg}
			}
			options.InputPath = args[index+1]
			index++
		case "--format":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "advise requires a value after --format"}
			}
			options.Format = args[index+1]
			index++
		case "--state":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "advise requires a repo-relative path after --state"}
			}
			options.StatePath = args[index+1]
			index++
		case "--comment":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "advise requires a repo-relative path after --comment"}
			}
			options.CommentPath = args[index+1]
			index++
		default:
			return options, &ParseError{Message: fmt.Sprintf("unknown advise argument %q", arg)}
		}
	}
	if strings.TrimSpace(options.InputPath) == "" {
		return options, &ParseError{Message: "advise requires --input <diff> in offline mode"}
	}
	if options.Format != "json" {
		return options, &ParseError{Message: "advise only supports --format json in this task slice"}
	}
	for _, item := range []struct {
		label string
		path  string
	}{
		{"advise --state", options.StatePath},
		{"advise --comment", options.CommentPath},
	} {
		if _, ok := configdoc.CleanRepoPath(item.path); !ok {
			return options, &ParseError{Message: item.label + " must be repo-relative"}
		}
	}
	return options, nil
}
