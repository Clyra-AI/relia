package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func ValidateSchemaContracts(root string, requiredSchemaFiles []string) *Error {
	for _, rel := range requiredSchemaFiles {
		content, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			if os.IsNotExist(err) {
				return artifactContractError("required schema is missing: "+rel, rel)
			}
			return internalError("could not read schema "+rel, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(content, &schema); err != nil {
			return artifactContractError("schema is not valid JSON: "+rel, rel)
		}
		if schema["type"] != "object" {
			return artifactContractError("schema must describe a JSON object: "+rel, rel)
		}
		required, ok := schema["required"].([]any)
		if !ok {
			return artifactContractError("schema missing required array: "+rel, rel)
		}
		if !containsRequiredString(required, "schema_version") {
			return artifactContractError("schema must require schema_version: "+rel, rel)
		}
		if !containsRequiredString(required, "metadata") {
			return artifactContractError("schema must require metadata: "+rel, rel)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			return artifactContractError("schema missing properties object: "+rel, rel)
		}
		if _, ok := properties["schema_version"]; !ok {
			return artifactContractError("schema missing schema_version property: "+rel, rel)
		}
		if _, ok := properties["metadata"]; !ok {
			return artifactContractError("schema missing metadata property: "+rel, rel)
		}
	}
	return nil
}

func containsRequiredString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
