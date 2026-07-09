package diffparse

import (
	"errors"
	"fmt"
	"testing"
)

func TestTouchedPathsParsesRepresentativeDiffForms(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name: "quoted spaces",
			content: `diff --git "a/docs/api guide.md" "b/docs/api guide.md"
--- "a/docs/api guide.md"
+++ "b/docs/api guide.md"
@@ -1 +1 @@
-old
+new
`,
			want: []string{"docs/api guide.md"},
		},
		{
			name: "rename without git prefixes",
			content: `diff --git foo bar.txt foo copy.txt
similarity index 100%
copy from foo bar.txt
copy to foo copy.txt
`,
			want: []string{"foo bar.txt", "foo copy.txt"},
		},
		{
			name: "ignore hunk body path lookalikes",
			content: `diff --git a/packages/search/query.py b/packages/search/query.py
--- a/packages/search/query.py
+++ b/packages/search/query.py
@@ -1,2 +1,4 @@
 def normalize_query(value):
-    return value.strip().lower()
++ packages/billing/invoice.py
+    normalized = value.strip().lower()
`,
			want: []string{"packages/search/query.py"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := TouchedPaths([]byte(test.content))
			if err != nil {
				t.Fatalf("TouchedPaths error: %v", err)
			}
			if fmt.Sprint(got) != fmt.Sprint(test.want) {
				t.Fatalf("paths = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestTouchedPathsReturnsSentinelForNoRepoPaths(t *testing.T) {
	_, err := TouchedPaths([]byte("not a diff\n"))
	if !errors.Is(err, ErrNoRepoRelativePaths) {
		t.Fatalf("error = %v, want ErrNoRepoRelativePaths", err)
	}
}

func TestTouchedPathsOrPlanFallsBackToPlanPaths(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Update packages/billing/invoice.py to use the billing clock fixture.
- Adjust docs/product/prd.md#serve-compile-assess-advise for the CLI contract.
- Ignore https://github.com/acme/billing-service/pull/142 citations.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{"docs/product/prd.md", "packages/billing/invoice.py"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestTouchedPathsOrPlanPreservesRootLevelPlanPaths(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Update README.md with the local assessment command.
- Keep go.mod pinned to the current module path.
- Touch internal/assess/assess.go for the shared assessment behavior.
- Ignore https://github.com/acme/relia/pull/164 review links.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{"README.md", "go.mod", "internal/assess/assess.go"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestTouchedPathsOrPlanRejectsGlobLikePlanTokens(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Run go test ./internal/... before shipping.
- Review cmd/relia/*.go for command wiring.
- Change internal/assess/assess.go for coverage semantics.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{"internal/assess/assess.go"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestTouchedPathsOrPlanRejectsProseAbbreviations(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- For example, e.g. no concrete repo file is named here.
- In other words, i.e. this prose should not look like a path.
- Keep README.md and package.json as concrete root-level paths.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{"README.md", "package.json"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestTouchedPathsOrPlanRejectsRemoteModulePathTokens(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Import github.com/Clyra-AI/relia/internal/assess in examples.
- Keep .github/workflows/validate.yml wired to the validator.
- Update docs/v1.0/assessment.md with plan guidance.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{".github/workflows/validate.yml", "docs/v1.0/assessment.md"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestTouchedPathsOrPlanRejectsSSHRemoteFragments(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Compare git@github.com:Clyra-AI/relia before editing README.md.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"README.md"}) {
		t.Fatalf("paths = %#v", paths)
	}

	_, _, err = TouchedPathsOrPlan([]byte("Compare git@github.com:Clyra-AI/relia only."))
	if !errors.Is(err, ErrNoRepoRelativePaths) {
		t.Fatalf("error = %v, want ErrNoRepoRelativePaths", err)
	}
}

func TestTouchedPathsOrPlanRejectsBareDomains(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Compare github.com and example.org before editing README.md.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"README.md"}) {
		t.Fatalf("paths = %#v", paths)
	}

	_, _, err = TouchedPathsOrPlan([]byte("Compare github.com and example.org only."))
	if !errors.Is(err, ErrNoRepoRelativePaths) {
		t.Fatalf("error = %v, want ErrNoRepoRelativePaths", err)
	}
}

func TestTouchedPathsOrPlanStripsOptionPathPrefixes(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Run relia assess --input=packages/billing/invoice.py before advice.
- Run relia assess --config=relia.yaml for root configuration coverage.
- Ignore --verbose because it is not a path-valued option.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{"packages/billing/invoice.py", "relia.yaml"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestTouchedPathsOrPlanRejectsEnvAssignmentPaths(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Run CONFIG=relia.yaml go test ./internal/assess.
- Run GOCACHE=/tmp/relia go test ./internal/advise.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{"internal/advise", "internal/assess"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestTouchedPathsOrPlanPreservesQuotedPathsWithSpaces(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Update ` + "`docs/api guide.md`" + ` with the operator-facing contract.
- Run relia assess --input="docs/release notes.md" before advisory output.
- Keep 'docs/single quote guide.md' in the same path extraction path.
- Keep "release notes.md" as a quoted root-level path with spaces.
- Do not record split fragments from quoted paths.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{"docs/api guide.md", "docs/release notes.md", "docs/single quote guide.md", "release notes.md"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestTouchedPathsOrPlanSplitsQuotedCommandSpans(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Run ` + "`relia assess --input packages/billing/invoice.py`" + ` before advisory output.
- Also run "relia assess --input=src/components/button.tsx" for UI coverage.
- Then run "./scripts/check packages/search/query.py" for the repo-local helper.
- Run "python3 scripts/check.py packages/billing/receipt.py" through the interpreter helper.
- Finally run "relia serve --tool coverage --paths=internal/assess/assess.go,internal/advise/advise.go".
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{"internal/advise/advise.go", "internal/assess/assess.go", "packages/billing/invoice.py", "packages/billing/receipt.py", "packages/search/query.py", "src/components/button.tsx"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestTouchedPathsOrPlanStripsQuotedLineSuffixes(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Update ` + "`README.md:42`" + ` for the quick-start note.
- Inspect "internal/diffparse/diffparse.go:164:8" before changing parser behavior.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{"README.md", "internal/diffparse/diffparse.go"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestTouchedPathsOrPlanRejectsGitHubShorthandRefs(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Compare against Clyra-AI/relia#142 before changing behavior.
- Update packages/billing/invoice.py with the covered implementation.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{"packages/billing/invoice.py"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}

	paths, _, err = TouchedPathsOrPlan([]byte("Compare Clyra-AI/relia#142 and docs/architecture.md#42."))
	if err != nil {
		t.Fatalf("fragment path should remain valid: %v", err)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"docs/architecture.md"}) {
		t.Fatalf("fragment paths = %#v", paths)
	}
}

func TestTouchedPathsOrPlanRejectsSlashDelimitedProse(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`Implementation plan:

- Keep CI/CD and read/write guidance aligned.
- Choose this and/or that while updating packages/billing/invoice.py.
- Mention 2026/07/09 and 7/9 as schedule dates, not paths.
- Mention T1/T2/T3 and FR16/FR19 as task chains, not paths.
- Preserve internal/assess package handling for directory-scoped rules.
- Preserve pkg/billing and src/components as generic directory-scoped paths.
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "plan" {
		t.Fatalf("input kind = %q, want plan", inputKind)
	}
	want := []string{"internal/assess", "packages/billing/invoice.py", "pkg/billing", "src/components"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}

	_, _, err = TouchedPathsOrPlan([]byte("Improve CI/CD, read/write, 2026/07/09, 7/9, T1/T2/T3, FR16/FR19, and and/or prose only."))
	if !errors.Is(err, ErrNoRepoRelativePaths) {
		t.Fatalf("error = %v, want ErrNoRepoRelativePaths", err)
	}
}

func TestTouchedPathsOrPlanKeepsUnifiedDiffKind(t *testing.T) {
	paths, inputKind, err := TouchedPathsOrPlan([]byte(`diff --git a/packages/search/query.py b/packages/search/query.py
--- a/packages/search/query.py
+++ b/packages/search/query.py
@@ -1 +1 @@
-old
+new
`))
	if err != nil {
		t.Fatalf("TouchedPathsOrPlan error: %v", err)
	}
	if inputKind != "diff" {
		t.Fatalf("input kind = %q, want diff", inputKind)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"packages/search/query.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}
