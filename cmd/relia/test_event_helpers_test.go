package main

import (
	"fmt"
	"strings"
)

func stringListField(event map[string]any, paths ...string) []string {
	for _, path := range paths {
		if value, ok := nestedTestField(event, path); ok {
			switch typed := value.(type) {
			case []any:
				var result []string
				for _, item := range typed {
					if converted := stringFromAny(item); converted != "" {
						result = append(result, converted)
					}
				}
				if len(result) > 0 {
					return result
				}
			case []string:
				if len(typed) > 0 {
					return typed
				}
			case string:
				if strings.TrimSpace(typed) != "" {
					return []string{strings.TrimSpace(typed)}
				}
			}
		}
	}
	return nil
}

func nestedTestField(event map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = event
	for _, part := range parts {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapping[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}
