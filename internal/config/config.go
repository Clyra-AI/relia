package config

import (
	"fmt"
	"strings"

	"github.com/Clyra-AI/relia/internal/yamlmini"
)

const DefaultFile = "relia.yaml"

func DefaultYAML(schemaVersion string, reliaVersion string) string {
	return fmt.Sprintf(`version: 1

artifacts:
  schema_version: "%s"
  relia_version: "%s"
  root: .relia
  commit_experiences: false

repo:
  provider: github
  remote: origin
  scopes: []

attribution:
  agent_authors: []
  coauthor_trailers:
    - Claude
    - Claude Code
  pr_labels:
    - agent-authored
  uncertain: exclude

outcomes:
  checks:
    required: []

privacy:
  local_only: true
  send_code: false
  send_diffs: false
  send_logs: false
  send_experience_records: false
  share_scope: private

redaction:
  schema_version: "%s"
  entropy_scan: true
  fail_closed: true
  standard_token_shapes: true

distill:
  embeddings: signature
  provider: none
  model: ""
  base_url: ""
  credential_env: ""
  max_cost_usd_per_run: 0
  input_cost_usd_per_1k_tokens: 0
  output_cost_usd_per_1k_tokens: 0
  review_required: true

models:
  local_manifest: .relia/models/manifest.json

serve:
  advisory_only: true

advise:
  enabled: true
  max_comments_per_pr: 1
  update_in_place: true
  reassess_debounce_minutes: 10
  min_confidence: 0.6

gate:
  enabled: false
`, schemaVersion, reliaVersion, schemaVersion)
}

func Ref(defaultPath string, scalar yamlmini.Scalar) string {
	return RefWithPath(defaultPath, scalar)
}

func RefWithPath(path string, scalar yamlmini.Scalar) string {
	if scalar.Line <= 0 {
		return path
	}
	return fmt.Sprintf("%s:%d", path, scalar.Line)
}

func PathRef(defaultPath string, document yamlmini.Document, path string) string {
	if scalar, ok := document.Scalars[path]; ok {
		return Ref(defaultPath, scalar)
	}
	if scalar, ok := document.Containers[path]; ok {
		return Ref(defaultPath, scalar)
	}
	if scalars := document.Lists[path]; len(scalars) > 0 {
		return Ref(defaultPath, scalars[0])
	}
	prefix := path + "."
	bestLine := 0
	for _, collection := range []map[string]yamlmini.Scalar{document.Scalars, document.Containers} {
		for key, scalar := range collection {
			if strings.HasPrefix(key, prefix) && (bestLine == 0 || scalar.Line < bestLine) {
				bestLine = scalar.Line
			}
		}
	}
	if bestLine > 0 {
		return Ref(defaultPath, yamlmini.Scalar{Line: bestLine})
	}
	return defaultPath
}
