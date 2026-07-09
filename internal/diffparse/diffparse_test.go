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
