package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	assessdoc "github.com/Clyra-AI/relia/internal/assess"
)

func TestParseUnifiedDiffTouchedPathsPreservesNoPrefixTopLevelABRename(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/foo.py b/foo.py
similarity index 100%
rename from a/foo.py
rename to b/foo.py
`), "rename-no-prefix-top-level-a-b.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"a/foo.py", "b/foo.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsPreservesRootSourceWhenRenameTargetStartsWithAPrefix(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/foo.py b/a/foo.py
similarity index 100%
rename from foo.py
rename to a/foo.py
`), "rename-root-to-a-prefix.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"a/foo.py", "foo.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsKeepsPriorRootPathWhenMetadataUsesAPrefix(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/.foo.py b/.foo.py
--- a/.foo.py
+++ b/.foo.py
@@ -1 +1 @@
-old
+new
diff --git a/.foo.py b/.foo.py
similarity index 100%
rename from a/.foo.py
rename to b/.foo.py
`), "root-path-before-no-prefix-a-prefix-rename.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{".foo.py", "a/.foo.py", "b/.foo.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsKeepsQuotedSpaces(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git "a/docs/api guide.md" "b/docs/api guide.md"
--- "a/docs/api guide.md"
+++ "b/docs/api guide.md"
@@ -1 +1 @@
-old
+new
`), "quoted.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"docs/api guide.md"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsPreservesQuotedTabPath(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git "a/foo\tbar.txt" "b/foo\tbar.txt"
--- "a/foo\tbar.txt"
+++ "b/foo\tbar.txt"
@@ -1 +1 @@
-old
+new
`), "quoted-tab.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if len(paths) != 1 || paths[0] != "foo\tbar.txt" {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsKeepsUnquotedSpaces(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/docs/api guide.md b/docs/api guide.md
--- a/docs/api guide.md
+++ b/docs/api guide.md
@@ -1 +1 @@
-old
+new
`), "unquoted-spaces.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"docs/api guide.md"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsKeepsNoPrefixUnquotedSpacesWithoutSplitTokens(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git foo bar.txt foo bar.txt
--- foo bar.txt
+++ foo bar.txt
@@ -1 +1 @@
-old
+new
`), "no-prefix-unquoted-spaces.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"foo bar.txt"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsPreservesNoPrefixDeleteLiteralAPrefix(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/foo.py a/foo.py
deleted file mode 100644
--- a/foo.py
+++ /dev/null
@@ -1 +0,0 @@
-old
`), "no-prefix-delete-a-prefix.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"a/foo.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsSupportsPlainUnifiedDiff(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`--- packages/billing/invoice.py
+++ packages/billing/invoice.py
@@ -1 +1 @@
-old
+new
`), "plain.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"packages/billing/invoice.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsPreservesPlainDiffLiteralAPrefix(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`--- a/foo.py
+++ a/foo.py
@@ -1 +1 @@
-old
+new
`), "plain-a-prefix.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"a/foo.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsPreservesNoPrefixGitLiteralAPrefix(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/foo.py a/foo.py
--- a/foo.py
+++ a/foo.py
@@ -1 +1 @@
-old
+new
`), "no-prefix-git-a-prefix.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"a/foo.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsSupportsNoPrefixModeOnlyHeader(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git foo.py foo.py
old mode 100644
new mode 100755
`), "mode-only-no-prefix.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"foo.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsSupportsNoPrefixModeOnlyHeaderWithSpaces(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git foo bar.txt foo bar.txt
old mode 100644
new mode 100755
`), "mode-only-no-prefix-spaces.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"foo bar.txt"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsPreservesNoPrefixModeOnlyLiteralAPrefix(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/foo.py a/foo.py
old mode 100644
new mode 100755
`), "mode-only-no-prefix-a-prefix.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"a/foo.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsHandlesAmbiguousPrefixedModeOnlyHeader(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/foo b/bar.txt b/foo b/bar.txt
old mode 100644
new mode 100755
`), "mode-only-prefixed-ambiguous-b-prefix.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"foo b/bar.txt"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsIgnoresPlainHeaderLookalikesInsideHunks(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/packages/search/query.py b/packages/search/query.py
--- a/packages/search/query.py
+++ b/packages/search/query.py
@@ -1,4 +1,5 @@
 def normalize_query(value):
--- packages/billing/invoice.py
+++ packages/billing/invoice.py
@@ -8,2 +9,3 @@
     return value.strip().lower()
`), "hunk-lookalike.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"packages/search/query.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsSkipsAmbiguousGitHeaderSeparator(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/foo b/bar.txt b/foo b/bar.txt
--- a/foo b/bar.txt
+++ b/foo b/bar.txt
@@ -1 +1 @@
-old
+new
`), "ambiguous-git-header.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"foo b/bar.txt"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsRecognizesSubsequentPlainFileHeaders(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`--- packages/search/query.py
+++ packages/search/query.py
@@ -1 +1 @@
-old
+new
--- packages/billing/invoice.py
+++ packages/billing/invoice.py
@@ -1 +1 @@
-old
+new
`), "multi-file-plain.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"packages/billing/invoice.py", "packages/search/query.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsPreservesRenameMetadataLiteralAPrefix(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git a/a/foo.py b/a/bar.py
similarity index 100%
rename from a/foo.py
rename to a/bar.py
`), "rename-literal-a.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"a/bar.py", "a/foo.py"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestParseUnifiedDiffTouchedPathsRecognizesNoPrefixCopyMetadata(t *testing.T) {
	paths, commandErr := parseUnifiedDiffTouchedPaths([]byte(`diff --git foo bar.txt foo copy.txt
similarity index 100%
copy from foo bar.txt
copy to foo copy.txt
`), "copy-no-prefix.diff")
	if commandErr != nil {
		t.Fatalf("parse diff: %v", commandErr)
	}
	if fmt.Sprint(paths) != fmt.Sprint([]string{"foo bar.txt", "foo copy.txt"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestNormalizeAssessmentScopePathDoesNotScanHistoryForExistingFile(t *testing.T) {
	tempDir := setupContractRepo(t)
	writeFileForTest(t, filepath.Join(tempDir, "packages", "billing", "invoice.py"), "def rollover_day(): pass\n")
	binDir := filepath.Join(tempDir, "bin")
	marker := filepath.Join(tempDir, "git-called")
	writeFileForTest(t, filepath.Join(binDir, "git"), "#!/bin/sh\necho called > "+marker+"\nexit 0\n")
	if err := os.Chmod(filepath.Join(binDir, "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	scopePath, directoryScope, ok := assessdoc.NormalizeScopePath(tempDir, "packages/billing/invoice.py")

	if !ok {
		t.Fatalf("scope path was rejected")
	}
	if scopePath != "packages/billing/invoice.py" {
		t.Fatalf("scope path = %q", scopePath)
	}
	if directoryScope {
		t.Fatalf("regular file scope was treated as directory scope")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("historical git scan ran for an existing regular file")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
