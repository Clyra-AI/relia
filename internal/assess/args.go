package assess

import (
	"fmt"
	"strings"
)

type CLIOptions struct {
	InputPath      string
	Format         string
	FormatExplicit bool
}

type ParseError struct {
	Message string
}

func (e ParseError) Error() string {
	return e.Message
}

func ParseArgs(args []string) (CLIOptions, *ParseError) {
	options := CLIOptions{Format: "json"}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--input", "--diff", "-i":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "assess requires a path after " + arg}
			}
			options.InputPath = args[index+1]
			index++
		case "--format":
			options.FormatExplicit = true
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "assess requires a value after --format"}
			}
			options.Format = args[index+1]
			index++
		default:
			return options, &ParseError{Message: fmt.Sprintf("unknown assess argument %q", arg)}
		}
	}
	if strings.TrimSpace(options.InputPath) == "" {
		return options, &ParseError{Message: "assess requires --input <diff> in offline mode"}
	}
	if options.Format != "json" {
		return options, &ParseError{Message: "assess only supports --format json in this task slice"}
	}
	return options, nil
}
