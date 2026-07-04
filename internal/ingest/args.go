package ingest

import (
	"fmt"
	"strings"
)

type CLIOptions struct {
	InputPath      string
	GitHubOutcomes bool
}

type ParseError struct {
	Message string
}

func (e ParseError) Error() string {
	return e.Message
}

func ParseArgs(args []string) (CLIOptions, *ParseError) {
	var options CLIOptions
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--input", "-i":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "ingest requires a path after --input"}
			}
			options.InputPath = args[index+1]
			index++
		case "--github-outcomes":
			options.GitHubOutcomes = true
		default:
			return options, &ParseError{Message: fmt.Sprintf("unknown ingest argument %q", arg)}
		}
	}
	if strings.TrimSpace(options.InputPath) == "" {
		return options, &ParseError{Message: "ingest requires --input <json-or-jsonl> in offline mode"}
	}
	return options, nil
}
