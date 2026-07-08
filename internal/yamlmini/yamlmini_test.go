package yamlmini

import (
	"reflect"
	"testing"
)

func TestParseDocumentRecordsNestedFieldsInListMapItems(t *testing.T) {
	document, err := ParseDocument(`repo:
  scopes:
    - prefix: packages/billing/
      checks:
        - pytest-billing
provenance:
  -
    pr: 141
    outcome: ci_failure
  - pr: 142
    outcome: revert
`)
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	scopes := document.ListMaps["repo.scopes"]
	if len(scopes) != 1 {
		t.Fatalf("repo.scopes list maps = %#v", scopes)
	}
	if got := scopes[0]["prefix"].Value; got != "packages/billing/" {
		t.Fatalf("scope prefix = %q", got)
	}
	if _, ok := scopes[0]["checks"]; !ok {
		t.Fatalf("scope list-map fields = %#v, want checks container", scopes[0])
	}
	if got := ListValues(document, "repo.scopes[0].checks"); !reflect.DeepEqual(got, []string{"pytest-billing"}) {
		t.Fatalf("scope checks = %#v", got)
	}

	provenance := document.ListMaps["provenance"]
	if len(provenance) != 2 {
		t.Fatalf("provenance list maps = %#v", provenance)
	}
	if got := provenance[0]["pr"].Value; got != "141" {
		t.Fatalf("first provenance pr = %q", got)
	}
	if got := provenance[1]["outcome"].Value; got != "revert" {
		t.Fatalf("second provenance outcome = %q", got)
	}
}

func TestParseDocumentHandlesCommentsQuotesAndBlockScalars(t *testing.T) {
	document, err := ParseDocument(`root:
  quoted: "value # not a comment"
  single: 'other # not a comment'
  plain: keep # remove
  block: |
    ignored: as scalar body
  next: after
`)
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	if got := document.Scalars["root.quoted"].Value; got != "value # not a comment" {
		t.Fatalf("quoted scalar = %q", got)
	}
	if got := document.Scalars["root.single"].Value; got != "other # not a comment" {
		t.Fatalf("single scalar = %q", got)
	}
	if got := document.Scalars["root.plain"].Value; got != "keep" {
		t.Fatalf("plain scalar = %q", got)
	}
	if got := document.Scalars["root.block"].Value; got != "|" {
		t.Fatalf("block scalar marker = %q", got)
	}
	if got := document.Scalars["root.next"].Value; got != "after" {
		t.Fatalf("next scalar = %q", got)
	}
}

func TestParseDocumentUnquotesListScalars(t *testing.T) {
	document, err := ParseDocument(`checks:
  - "test #1"
  - 'build: linux'
`)
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	if got := ListValues(document, "checks"); !reflect.DeepEqual(got, []string{"test #1", "build: linux"}) {
		t.Fatalf("checks = %#v", got)
	}
}

func TestListValuesWithMapFieldsReturnsUniqueValues(t *testing.T) {
	document, err := ParseDocument(`attribution:
  agent_authors:
    - login: codex
    - login: codex
    - login: factoryd
    - empty:
`)
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	got := ListValuesWithMapFields(document, "attribution.agent_authors", "login")
	want := []string{"login: codex", "login: factoryd", "empty:", "codex", "factoryd"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListValuesWithMapFields = %#v, want %#v", got, want)
	}
}

func TestHasPathMatchesDescendantPaths(t *testing.T) {
	document, err := ParseDocument(`memory:
  rules:
    - id: avoid-1
      scope:
        paths:
          - cmd/relia/main.go
`)
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	for _, path := range []string{"memory", "memory.rules", "memory.rules[0].scope.paths"} {
		if !HasPath(document, path) {
			t.Fatalf("HasPath(%q) = false", path)
		}
	}
	if HasPath(document, "memory.missing") {
		t.Fatalf("HasPath returned true for missing path")
	}
}
