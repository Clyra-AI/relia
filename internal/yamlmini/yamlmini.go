package yamlmini

import (
	"fmt"
	"strings"
)

type Scalar struct {
	Value string
	Line  int
}

type Document struct {
	Scalars    map[string]Scalar
	Lists      map[string][]Scalar
	ListMaps   map[string][]map[string]Scalar
	Containers map[string]Scalar
}

type context struct {
	Path       string
	ListItem   bool
	ListParent string
	ListIndex  int
}

func ParseDocument(content string) (Document, error) {
	document := Document{
		Scalars:    map[string]Scalar{},
		Lists:      map[string][]Scalar{},
		ListMaps:   map[string][]map[string]Scalar{},
		Containers: map[string]Scalar{},
	}
	var stack []context
	blockScalarIndent := -1
	lines := strings.Split(content, "\n")
	for index, raw := range lines {
		lineNumber := index + 1
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := LeadingSpaces(raw)
		if blockScalarIndent >= 0 {
			if indent > blockScalarIndent {
				continue
			}
			blockScalarIndent = -1
		}
		if indent%2 != 0 {
			return document, fmt.Errorf("invalid YAML indentation at line %d", lineNumber)
		}
		depth := indent / 2
		trimmed := stripInlineComment(strings.TrimSpace(raw))
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		if trimmed == "-" || strings.HasPrefix(trimmed, "- ") {
			if depth > len(stack) {
				return document, fmt.Errorf("list item without parent at line %d", lineNumber)
			}
			parent := parentPath(stack, depth)
			if parent == "" {
				return document, fmt.Errorf("top-level lists are not supported at line %d", lineNumber)
			}
			itemValue := ""
			if strings.HasPrefix(trimmed, "- ") {
				itemValue = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			}
			document.Lists[parent] = append(document.Lists[parent], Scalar{
				Value: itemValue,
				Line:  lineNumber,
			})
			itemIndex := len(document.Lists[parent]) - 1
			stack = append(stack[:depth], context{
				Path:       fmt.Sprintf("%s[%d]", parent, itemIndex),
				ListItem:   true,
				ListParent: parent,
				ListIndex:  itemIndex,
			})
			if key, value, ok := cutMapping(itemValue); ok {
				scalarValue := unquoteScalar(value)
				recordListMapScalar(document, parent, itemIndex, key, Scalar{Value: scalarValue, Line: lineNumber})
				if scalarValue == ">" || scalarValue == "|" {
					blockScalarIndent = indent
				}
			}
			continue
		}
		key, value, ok := cutMapping(trimmed)
		if !ok {
			return document, fmt.Errorf("expected key/value pair at line %d", lineNumber)
		}
		if key == "" {
			return document, fmt.Errorf("empty key at line %d", lineNumber)
		}
		if depth > len(stack) {
			return document, fmt.Errorf("missing parent for %s at line %d", key, lineNumber)
		}
		path := key
		if parent := parentPath(stack, depth); parent != "" {
			path = parent + "." + key
		}
		stack = append(stack[:depth], context{Path: path})
		if value == "" {
			document.Containers[path] = Scalar{Line: lineNumber}
			if listParent, listIndex, itemPath, ok := nearestListItem(stack[:depth]); ok {
				field := strings.TrimPrefix(path, itemPath+".")
				recordListMapScalar(document, listParent, listIndex, field, Scalar{Line: lineNumber})
			}
			continue
		}
		scalarValue := unquoteScalar(value)
		document.Scalars[path] = Scalar{Value: scalarValue, Line: lineNumber}
		if listParent, listIndex, itemPath, ok := nearestListItem(stack[:depth]); ok {
			field := strings.TrimPrefix(path, itemPath+".")
			recordListMapScalar(document, listParent, listIndex, field, Scalar{Value: scalarValue, Line: lineNumber})
		}
		if scalarValue == ">" || scalarValue == "|" {
			blockScalarIndent = indent
		}
	}
	return document, nil
}

func ListValues(document Document, path string) []string {
	scalars := document.Lists[path]
	values := make([]string, 0, len(scalars))
	for _, scalar := range scalars {
		if strings.TrimSpace(scalar.Value) != "" {
			values = append(values, scalar.Value)
		}
	}
	return values
}

func ListValuesWithMapFields(document Document, path string, fields ...string) []string {
	values := ListValues(document, path)
	for _, mapping := range document.ListMaps[path] {
		for _, field := range fields {
			scalar, ok := mapping[field]
			if !ok || strings.TrimSpace(scalar.Value) == "" {
				continue
			}
			values = append(values, scalar.Value)
		}
	}
	return uniqueStrings(values)
}

func HasPath(document Document, path string) bool {
	if _, ok := document.Scalars[path]; ok {
		return true
	}
	if _, ok := document.Containers[path]; ok {
		return true
	}
	if _, ok := document.Lists[path]; ok {
		return true
	}
	if _, ok := document.ListMaps[path]; ok {
		return true
	}
	prefix := path + "."
	indexedPrefix := path + "["
	for key := range document.Scalars {
		if strings.HasPrefix(key, prefix) || strings.HasPrefix(key, indexedPrefix) {
			return true
		}
	}
	for key := range document.Containers {
		if strings.HasPrefix(key, prefix) || strings.HasPrefix(key, indexedPrefix) {
			return true
		}
	}
	for key := range document.Lists {
		if strings.HasPrefix(key, prefix) || strings.HasPrefix(key, indexedPrefix) {
			return true
		}
	}
	return false
}

func LeadingSpaces(value string) int {
	count := 0
	for _, r := range value {
		if r != ' ' {
			return count
		}
		count++
	}
	return count
}

func parentPath(stack []context, depth int) string {
	if depth <= 0 {
		return ""
	}
	return stack[depth-1].Path
}

func cutMapping(value string) (string, string, bool) {
	key, rest, ok := strings.Cut(value, ":")
	if !ok {
		return "", "", false
	}
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	return key, strings.TrimSpace(rest), true
}

func nearestListItem(stack []context) (string, int, string, bool) {
	for index := len(stack) - 1; index >= 0; index-- {
		context := stack[index]
		if context.ListItem {
			return context.ListParent, context.ListIndex, context.Path, true
		}
	}
	return "", 0, "", false
}

func recordListMapScalar(document Document, parent string, index int, key string, scalar Scalar) {
	for len(document.ListMaps[parent]) <= index {
		document.ListMaps[parent] = append(document.ListMaps[parent], map[string]Scalar{})
	}
	document.ListMaps[parent][index][key] = scalar
}

func stripInlineComment(value string) string {
	inSingle := false
	inDouble := false
	for index, r := range value {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble && (index == 0 || value[index-1] == ' ' || value[index-1] == '\t') {
				return strings.TrimSpace(value[:index])
			}
		}
	}
	return value
}

func unquoteScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
