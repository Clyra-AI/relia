package memory

import (
	"strings"

	"github.com/Clyra-AI/relia/internal/yamlmini"
)

// ResolvedRuleStatement returns the user-authored rule statement, including
// rendered YAML block scalars, so validation, summaries, and serving agree.
func ResolvedRuleStatement(document yamlmini.Document, content string) (string, bool) {
	scalar, ok := document.Scalars["statement"]
	if !ok {
		return "", false
	}
	style, ok := statementBlockScalarStyle(content, scalar.Line)
	if !ok {
		return strings.TrimSpace(scalar.Value), true
	}
	return strings.TrimSpace(renderStatementBlockScalar(content, scalar.Line, style)), true
}

func resolvedRuleStatement(document yamlmini.Document, content string) (string, bool) {
	return ResolvedRuleStatement(document, content)
}

func statementBlockScalarStyle(content string, lineNumber int) (byte, bool) {
	if lineNumber <= 0 {
		return 0, false
	}
	lines := strings.Split(content, "\n")
	if lineNumber > len(lines) {
		return 0, false
	}
	line := strings.TrimSpace(lines[lineNumber-1])
	if !strings.HasPrefix(line, "statement:") {
		return 0, false
	}
	value := strings.TrimSpace(strings.TrimPrefix(line, "statement:"))
	if hash := strings.Index(value, " #"); hash >= 0 {
		value = strings.TrimSpace(value[:hash])
	}
	switch value {
	case ">":
		return '>', true
	case "|":
		return '|', true
	default:
		return 0, false
	}
}

func renderStatementBlockScalar(content string, lineNumber int, style byte) string {
	lines := strings.Split(content, "\n")
	if lineNumber <= 0 || lineNumber >= len(lines)+1 {
		return ""
	}
	baseIndent := yamlmini.LeadingSpaces(lines[lineNumber-1])
	bodyLines := statementBlockBodyLines(lines[lineNumber:], baseIndent)
	if len(bodyLines) == 0 {
		return ""
	}
	trimmed := trimEmptyBlockEdges(stripBlockIndent(bodyLines, baseIndent))
	if style == '|' {
		return strings.Join(trimmed, "\n")
	}
	return foldStatementBlock(trimmed)
}

func statementBlockBodyLines(lines []string, baseIndent int) []string {
	body := []string{}
	for _, line := range lines {
		if strings.TrimSpace(line) != "" && yamlmini.LeadingSpaces(line) <= baseIndent {
			break
		}
		body = append(body, line)
	}
	return body
}

func stripBlockIndent(lines []string, baseIndent int) []string {
	contentIndent := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := yamlmini.LeadingSpaces(line)
		if indent <= baseIndent {
			continue
		}
		if contentIndent == 0 || indent < contentIndent {
			contentIndent = indent
		}
	}
	if contentIndent == 0 {
		return nil
	}
	stripped := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			stripped = append(stripped, "")
			continue
		}
		if len(line) >= contentIndent {
			stripped = append(stripped, line[contentIndent:])
			continue
		}
		stripped = append(stripped, strings.TrimSpace(line))
	}
	return stripped
}

func trimEmptyBlockEdges(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func foldStatementBlock(lines []string) string {
	var builder strings.Builder
	previousBlank := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if builder.Len() > 0 && !previousBlank {
				builder.WriteString("\n")
			}
			previousBlank = true
			continue
		}
		if builder.Len() > 0 {
			if previousBlank {
				builder.WriteString("\n")
			} else {
				builder.WriteString(" ")
			}
		}
		builder.WriteString(trimmed)
		previousBlank = false
	}
	return builder.String()
}
