package serve

import (
	"fmt"
	"strings"
)

type ErrorKind string

const (
	ErrorKindUsage      ErrorKind = "usage"
	ErrorKindDependency ErrorKind = "dependency"
)

type Options struct {
	Format    string
	Tool      string
	Context   string
	Paths     []string
	InputPath string
}

type ParseError struct {
	Kind      ErrorKind
	Message   string
	Reference string
}

func (e ParseError) Error() string {
	return e.Message
}

func ParseArgs(args []string) (Options, *ParseError) {
	options := Options{Format: "json"}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--format":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("serve requires a value after --format")
			}
			options.Format = args[index+1]
			index++
		case "--tool":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("serve requires a value after --tool")
			}
			options.Tool = args[index+1]
			index++
		case "--context":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("serve requires a value after --context")
			}
			options.Context = args[index+1]
			index++
		case "--paths":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("serve requires a value after --paths")
			}
			paths := splitPaths(args[index+1])
			if len(paths) == 0 {
				return options, usageError("serve --paths must name at least one repo-relative path")
			}
			if !allRepoRelative(paths) {
				return options, usageError("serve --paths values must be repo-relative")
			}
			options.Paths = append(options.Paths, paths...)
			index++
		case "--input", "--diff", "-i":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, usageError("serve requires a path after " + arg)
			}
			options.InputPath = args[index+1]
			index++
		case "--listen", "--http", "--hosted":
			return options, &ParseError{
				Kind:      ErrorKindDependency,
				Message:   "hosted or network serve transports are outside the MVP default and require explicit network approval",
				Reference: "docs/product/prd.md#serve-and-advise",
			}
		default:
			return options, usageError(fmt.Sprintf("unknown serve argument %q", arg))
		}
	}
	if options.Format != "json" {
		return options, usageError("serve only supports --format json in this task slice")
	}
	switch options.Tool {
	case "", "recall", "assess", "coverage":
	default:
		return options, usageError("serve --tool must be one of recall, assess, or coverage")
	}
	if options.Tool == "recall" && strings.TrimSpace(options.Context) == "" && len(options.Paths) == 0 {
		return options, usageError("serve recall requires --context or --paths")
	}
	if options.Tool == "coverage" && len(options.Paths) == 0 {
		return options, usageError("serve coverage requires --paths")
	}
	if options.Tool == "assess" && strings.TrimSpace(options.InputPath) == "" {
		return options, usageError("serve assess requires --input <diff>")
	}
	if options.Tool == "" && (strings.TrimSpace(options.Context) != "" || len(options.Paths) > 0 || strings.TrimSpace(options.InputPath) != "") {
		return options, usageError("serve tool arguments require --tool recall, assess, or coverage")
	}
	return options, nil
}

func usageError(message string) *ParseError {
	return &ParseError{Kind: ErrorKindUsage, Message: message}
}

func splitPaths(value string) []string {
	var paths []string
	for _, part := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			paths = append(paths, trimmed)
		}
	}
	return paths
}

func allRepoRelative(paths []string) bool {
	for _, path := range paths {
		if _, ok := cleanRepoPath(path); !ok {
			return false
		}
	}
	return true
}
