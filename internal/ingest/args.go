package ingest

import (
	"fmt"
	"strconv"
	"strings"
)

type CLIOptions struct {
	InputPath        string
	GitHubOutcomes   bool
	GitHubLive       bool
	GitHubRepo       string
	GitHubPulls      []int
	GitHubTokenEnv   string
	GitHubTokenScope string
	AllowNetwork     bool
	AllowCredentials bool
	HumanApproved    bool
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
		case "--github-live":
			options.GitHubLive = true
		case "--repo", "--github-repo":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "ingest --github-live requires a value after " + arg}
			}
			options.GitHubRepo = args[index+1]
			index++
		case "--pr", "--github-pr":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "ingest --github-live requires a pull request number after " + arg}
			}
			number, err := strconv.Atoi(strings.TrimSpace(args[index+1]))
			if err != nil || number < 1 {
				return options, &ParseError{Message: "ingest --github-live pull request numbers must be positive integers"}
			}
			options.GitHubPulls = append(options.GitHubPulls, number)
			index++
		case "--github-token-env":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "ingest --github-live requires a value after --github-token-env"}
			}
			options.GitHubTokenEnv = args[index+1]
			index++
		case "--github-token-scope":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "ingest --github-live requires a value after --github-token-scope"}
			}
			options.GitHubTokenScope = args[index+1]
			index++
		case "--allow-network":
			options.AllowNetwork = true
		case "--allow-credentials":
			options.AllowCredentials = true
		case "--human-approved":
			options.HumanApproved = true
		default:
			return options, &ParseError{Message: fmt.Sprintf("unknown ingest argument %q", arg)}
		}
	}
	if options.GitHubLive {
		if options.GitHubOutcomes || strings.TrimSpace(options.InputPath) != "" {
			return options, &ParseError{Message: "ingest --github-live cannot be combined with --github-outcomes or --input; use --github-outcomes --input <path> for offline replay"}
		}
		if strings.TrimSpace(options.GitHubRepo) == "" {
			return options, &ParseError{Message: "ingest --github-live requires --repo <owner/repo>"}
		}
		if len(options.GitHubPulls) == 0 {
			return options, &ParseError{Message: "ingest --github-live requires at least one --pr <number>"}
		}
		return options, nil
	}
	if options.hasGitHubLiveOnlyArgs() {
		return options, &ParseError{Message: "github live arguments require --github-live"}
	}
	if strings.TrimSpace(options.InputPath) == "" {
		return options, &ParseError{Message: "ingest requires --input <json-or-jsonl> in offline mode"}
	}
	return options, nil
}

func (options CLIOptions) hasGitHubLiveOnlyArgs() bool {
	return strings.TrimSpace(options.GitHubRepo) != "" ||
		len(options.GitHubPulls) > 0 ||
		strings.TrimSpace(options.GitHubTokenEnv) != "" ||
		strings.TrimSpace(options.GitHubTokenScope) != "" ||
		options.AllowNetwork ||
		options.AllowCredentials ||
		options.HumanApproved
}
