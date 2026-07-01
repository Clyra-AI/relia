package backtest

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type Options struct {
	Window         string
	Format         string
	FormatExplicit bool
	BaselinePath   string
	ReportDir      string
	SaveBaseline   bool
}

type ParseError struct {
	Message string
}

func (e ParseError) Error() string {
	return e.Message
}

func ParseArgs(args []string) (Options, *ParseError) {
	options := Options{
		Window:       "180d",
		Format:       "json",
		BaselinePath: ".relia/baselines/error-recurrence-baseline.json",
		ReportDir:    ".relia/reports",
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--window":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "backtest requires a value after --window"}
			}
			options.Window = args[index+1]
			index++
		case "--format":
			options.FormatExplicit = true
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "backtest requires a value after --format"}
			}
			options.Format = args[index+1]
			index++
		case "--baseline":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "backtest requires a repo-relative path after --baseline"}
			}
			options.BaselinePath = args[index+1]
			index++
		case "--report-dir":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "backtest requires a repo-relative path after --report-dir"}
			}
			options.ReportDir = args[index+1]
			index++
		case "--save-baseline":
			options.SaveBaseline = true
		default:
			return options, &ParseError{Message: fmt.Sprintf("unknown backtest argument %q", arg)}
		}
	}
	if options.Format != "json" {
		return options, &ParseError{Message: "backtest only supports --format json in this task slice"}
	}
	if _, parseErr := ParseWindowDays(options.Window); parseErr != nil {
		return options, parseErr
	}
	if _, ok := cleanRepoPath(options.BaselinePath); !ok {
		return options, &ParseError{Message: "backtest --baseline must be a repo-relative path"}
	}
	if _, ok := cleanRepoPath(options.ReportDir); !ok {
		return options, &ParseError{Message: "backtest --report-dir must be a repo-relative path"}
	}
	return options, nil
}

func ParseWindowDays(value string) (int, *ParseError) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if !strings.HasSuffix(trimmed, "d") {
		return 0, &ParseError{Message: "backtest --window must use a day duration such as 180d"}
	}
	days, err := strconv.Atoi(strings.TrimSuffix(trimmed, "d"))
	if err != nil || days <= 0 {
		return 0, &ParseError{Message: "backtest --window must be a positive day duration such as 180d"}
	}
	return days, nil
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
