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
	Format string
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
	return options, nil
}

func usageError(message string) *ParseError {
	return &ParseError{Kind: ErrorKindUsage, Message: message}
}
