package diffparse

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func TestLexPlanTextRetainsSpansAndClassificationContext(t *testing.T) {
	input := "Run `python3 scripts/check.py packages/billing/invoice.py` with --config=relia.yaml; ignore CONFIG=secret.yaml and T14/T14.1."
	tokens := LexPlanText(input)
	classes := map[PlanTokenClass]int{}
	for _, token := range tokens {
		classes[token.Class]++
		if token.Start < 0 || token.End < token.Start || token.End > len(input) {
			t.Fatalf("invalid token span: %#v", token)
		}
		if token.Text != input[token.Start:token.End] && token.Class != PlanTokenOptionValue {
			t.Fatalf("token text %q does not match source span %q", token.Text, input[token.Start:token.End])
		}
	}
	if classes[PlanTokenCommandSpan] != 1 {
		t.Fatalf("command spans = %d, want 1; tokens=%#v", classes[PlanTokenCommandSpan], tokens)
	}
	if classes[PlanTokenOptionValue] != 1 {
		t.Fatalf("option values = %d, want 1; tokens=%#v", classes[PlanTokenOptionValue], tokens)
	}
	if classes[PlanTokenEnvironmentAssignment] != 1 {
		t.Fatalf("env assignments = %d, want 1; tokens=%#v", classes[PlanTokenEnvironmentAssignment], tokens)
	}
	if classes[PlanTokenTaskID] != 1 {
		t.Fatalf("task IDs = %d, want 1; tokens=%#v", classes[PlanTokenTaskID], tokens)
	}
	command := firstPlanToken(tokens, PlanTokenCommandSpan)
	childClasses := map[PlanTokenClass]int{}
	for _, child := range command.Children {
		childClasses[child.Class]++
	}
	if childClasses[PlanTokenCommandHelper] != 2 || childClasses[PlanTokenUnquotedRepoPath] != 1 {
		t.Fatalf("command child classes = %#v, want two helpers and one target path", childClasses)
	}
}

func TestPlanPathsCorpus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "prose only", input: "Improve reliability and validation before release."},
		{name: "unavailable markers", input: "Paths are N/A, n/a, unavailable, and none."},
		{name: "remote and domain", input: "Compare https://example.com/a/b, github.com/acme/repo, and example.org."},
		{name: "task IDs and dates", input: "T14/T14.1 and FR3/FR4 are reviewed on 2026/07/09."},
		{name: "command targets", input: "Run python3 scripts/check.py internal/assess/assess.go and update README.md.", want: []string{"README.md", "internal/assess/assess.go"}},
		{name: "quoted local helper command target", input: "Run `./tools/check packages/foo.go`.", want: []string{"packages/foo.go"}},
		{name: "quoted extensionless local helper command target", input: "Run `tools/check packages/foo.go`.", want: []string{"packages/foo.go"}},
		{name: "quoted path", input: "Update `docs/release notes.md:42`.", want: []string{"docs/release notes.md"}},
		{name: "quoted path list preserves first path", input: `Update "README.md docs/product/prd.md".`, want: []string{"README.md", "docs/product/prd.md"}},
		{name: "planned new path", input: "Create internal/attribution/new_rule.go and docs/product/rollback.md.", want: []string{"docs/product/rollback.md", "internal/attribution/new_rule.go"}},
		{name: "structured field policy", input: `{"allowed_paths":["internal/assess/new.go"],"forbidden_paths":["secrets/key.txt"],"validation_commands":["go test ./internal/assess"],"summary":"Update README.md"}`, want: []string{"README.md", "internal/assess/new.go"}},
		{name: "structured path string list", input: `{"allowed_paths":"README.md docs/product/prd.md"}`, want: []string{"README.md", "docs/product/prd.md"}},
		{name: "structured path string with spaces", input: `{"allowed_paths":"docs/release notes.md"}`, want: []string{"docs/release notes.md"}},
		{name: "structured root path string with spaces", input: `{"allowed_paths":"release notes.md"}`, want: []string{"release notes.md"}},
		{name: "structured root path string with path-like first word", input: `{"allowed_paths":"README draft.md"}`, want: []string{"README draft.md"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := PlanPaths([]byte(test.input))
			if len(test.want) == 0 {
				if !errors.Is(err, ErrNoRepoRelativePaths) {
					t.Fatalf("PlanPaths error = %v, paths=%#v, want ErrNoRepoRelativePaths", err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("PlanPaths error: %v", err)
			}
			if fmt.Sprint(got) != fmt.Sprint(test.want) {
				t.Fatalf("paths = %#v, want %#v", got, test.want)
			}
		})
	}
}

func FuzzPlanPathsProseOnly(f *testing.F) {
	f.Add("improve validation reliability")
	f.Add("review the current behavior before release")
	f.Fuzz(func(t *testing.T, raw string) {
		var prose strings.Builder
		for _, char := range raw {
			if unicode.IsLetter(char) || unicode.IsSpace(char) {
				prose.WriteRune(char)
			} else {
				prose.WriteByte(' ')
			}
		}
		paths, err := PlanPaths([]byte(prose.String()))
		if len(paths) != 0 || !errors.Is(err, ErrNoRepoRelativePaths) {
			t.Fatalf("prose-only input produced paths %#v, error %v", paths, err)
		}
	})
}

func FuzzPlanPathsQuotedPathRoundTrip(f *testing.F) {
	f.Add("release notes")
	f.Add("api guide v2")
	f.Fuzz(func(t *testing.T, raw string) {
		name := safePlanPathSegment(raw)
		if name == "" {
			return
		}
		want := filepath.ToSlash(filepath.Join("docs", name+".md"))
		paths, err := PlanPaths([]byte("Update `" + want + "` before release."))
		if err != nil || fmt.Sprint(paths) != fmt.Sprint([]string{want}) {
			t.Fatalf("quoted path round trip: paths=%#v error=%v want=%q", paths, err, want)
		}
	})
}

func firstPlanToken(tokens []PlanToken, class PlanTokenClass) PlanToken {
	for _, token := range tokens {
		if token.Class == class {
			return token
		}
	}
	return PlanToken{}
}

func safePlanPathSegment(raw string) string {
	var value strings.Builder
	for _, char := range raw {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == ' ' || char == '-' || char == '_' {
			value.WriteRune(char)
		}
		if value.Len() >= 48 {
			break
		}
	}
	return strings.TrimSpace(value.String())
}
