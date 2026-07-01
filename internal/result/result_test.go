package result

import (
	"testing"
	"time"
)

func TestPassBuildsStableEnvelope(t *testing.T) {
	start := time.Now().Add(-25 * time.Millisecond)

	got := Pass("check", "check", "ready", start, nil, BuildOptions{
		SchemaVersion:           "1.0",
		ReliaVersion:            "0.0.0-test",
		SuccessExitCode:         0,
		RedactionSafetyExitCode: 6,
	})

	if got.ObjectType != ObjectType {
		t.Fatalf("object type = %q", got.ObjectType)
	}
	if got.SchemaVersion != "1.0" || got.Metadata["schema_version"] != "1.0" {
		t.Fatalf("schema version not propagated: %#v", got)
	}
	if got.Metadata["relia_version"] != "0.0.0-test" {
		t.Fatalf("relia version metadata = %#v", got.Metadata)
	}
	if got.Status != "pass" || got.ExitCode != 0 {
		t.Fatalf("status/exit code = %s/%d", got.Status, got.ExitCode)
	}
	if got.Data["message"] != "ready" {
		t.Fatalf("message data = %#v", got.Data)
	}
	if len(got.Errors) != 0 || len(got.Artifacts) != 0 || len(got.Warnings) != 0 {
		t.Fatalf("empty slices not initialized: %#v", got)
	}
	if got.RedactionStatus != "not_applicable" {
		t.Fatalf("redaction status = %q", got.RedactionStatus)
	}
	if got.DurationMS < 0 {
		t.Fatalf("duration = %d", got.DurationMS)
	}
}

func TestErrorMarksRedactionSafetyFailedClosed(t *testing.T) {
	commandErr := &CommandError{
		Type:     "redaction_safety_failed",
		Message:  "blocked",
		ExitCode: 6,
		Ref:      "relia.yaml",
	}

	got := Error("ingest", "ingest", commandErr, time.Now(), BuildOptions{
		SchemaVersion:           "1.0",
		ReliaVersion:            "0.0.0-test",
		SuccessExitCode:         0,
		RedactionSafetyExitCode: 6,
	})

	if got.Status != "error" || got.ExitCode != 6 {
		t.Fatalf("status/exit code = %s/%d", got.Status, got.ExitCode)
	}
	if len(got.Errors) != 1 || got.Errors[0].Message != "blocked" {
		t.Fatalf("errors = %#v", got.Errors)
	}
	if got.RedactionStatus != "failed_closed" {
		t.Fatalf("redaction status = %q", got.RedactionStatus)
	}
}

func TestErrorWithDataPreservesPayload(t *testing.T) {
	data := map[string]any{"provider": "local"}
	commandErr := &CommandError{Type: "dependency_error", Message: "missing", ExitCode: 8}

	got := ErrorWithData("distill", "distill", commandErr, time.Now(), data, BuildOptions{
		SchemaVersion:           "1.0",
		ReliaVersion:            "0.0.0-test",
		SuccessExitCode:         0,
		RedactionSafetyExitCode: 6,
	})

	if got.Data["provider"] != "local" {
		t.Fatalf("data = %#v", got.Data)
	}
}
