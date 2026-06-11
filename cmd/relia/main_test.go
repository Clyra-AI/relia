package main

import (
	"bytes"
	"testing"
)

func TestRunPrintsAppName(t *testing.T) {
	var stdout bytes.Buffer

	if err := run(&stdout); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if got, want := stdout.String(), "relia\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestExitCode(t *testing.T) {
	if got := exitCode(nil); got != 0 {
		t.Fatalf("exitCode(nil) = %d, want 0", got)
	}
	if got := exitCode(bytes.ErrTooLarge); got != 1 {
		t.Fatalf("exitCode(error) = %d, want 1", got)
	}
}
