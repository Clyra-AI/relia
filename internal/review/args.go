package review

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Options struct {
	Action     string
	Rule       string
	Label      string
	Statement  string
	Reason     string
	ScopePaths []string
}

type ParseError struct {
	Message string
}

func (e ParseError) Error() string {
	return e.Message
}

func ParseArgs(args []string) (Options, *ParseError) {
	var options Options
	if len(args) > 0 {
		switch args[0] {
		case "approve", "edit", "reject":
			options.Action = args[0]
			args = args[1:]
		}
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--rule":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "review requires a rule id or path after --rule"}
			}
			options.Rule = args[index+1]
			index++
		case "--label":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "review requires accepted, suggested, or needs_user_input after --label"}
			}
			options.Label = args[index+1]
			index++
		case "--statement":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "review edit requires a statement after --statement"}
			}
			options.Statement = args[index+1]
			index++
		case "--reason":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "review reject requires a reason after --reason"}
			}
			options.Reason = args[index+1]
			index++
		case "--scope-path":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, &ParseError{Message: "review edit requires a repo-relative path after --scope-path"}
			}
			options.ScopePaths = append(options.ScopePaths, args[index+1])
			index++
		default:
			return options, &ParseError{Message: fmt.Sprintf("unknown review argument %q", arg)}
		}
	}
	if options.Action == "" {
		options.Action = "label"
	}
	if strings.TrimSpace(options.Rule) == "" {
		return options, &ParseError{Message: "review requires --rule <id-or-path>"}
	}
	hasEditInput := strings.TrimSpace(options.Statement) != "" || len(options.ScopePaths) > 0
	if options.Action != "edit" && hasEditInput {
		return options, &ParseError{Message: "review --statement and --scope-path require review edit"}
	}
	if options.Action != "reject" && strings.TrimSpace(options.Reason) != "" {
		return options, &ParseError{Message: "review --reason requires review reject"}
	}
	switch options.Action {
	case "approve":
		if options.Label != "" && options.Label != "accepted" {
			return options, &ParseError{Message: "review approve can only use review label accepted"}
		}
		options.Label = "accepted"
	case "reject":
		if strings.TrimSpace(options.Reason) == "" {
			return options, &ParseError{Message: "review reject requires --reason <text>"}
		}
		if options.Label != "" && options.Label != "needs_user_input" {
			return options, &ParseError{Message: "review reject can only use review label needs_user_input"}
		}
		options.Label = "needs_user_input"
	case "edit":
		if strings.TrimSpace(options.Statement) == "" && len(options.ScopePaths) == 0 {
			return options, &ParseError{Message: "review edit requires --statement or --scope-path"}
		}
		if options.Label == "" {
			options.Label = "suggested"
		}
		if options.Label == "accepted" {
			return options, &ParseError{Message: "review edit keeps a rule candidate; run review approve after editing"}
		}
	case "label":
		if options.Label == "" {
			options.Label = "accepted"
		}
	default:
		return options, &ParseError{Message: "review action must be approve, edit, reject, or omitted for --label"}
	}
	switch options.Label {
	case "accepted", "suggested", "needs_user_input":
	default:
		return options, &ParseError{Message: "review --label must be accepted, suggested, or needs_user_input"}
	}
	for _, scopePath := range options.ScopePaths {
		if _, ok := cleanRepoPath(scopePath); !ok {
			return options, &ParseError{Message: "review --scope-path must be repo-relative"}
		}
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
