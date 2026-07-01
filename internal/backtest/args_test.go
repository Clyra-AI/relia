package backtest

import "testing"

func TestParseArgsDefaults(t *testing.T) {
	options, parseErr := ParseArgs(nil)
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Window != "180d" {
		t.Fatalf("Window = %q, want 180d", options.Window)
	}
	if options.Format != "json" {
		t.Fatalf("Format = %q, want json", options.Format)
	}
	if options.BaselinePath != ".relia/baselines/error-recurrence-baseline.json" {
		t.Fatalf("BaselinePath = %q", options.BaselinePath)
	}
	if options.ReportDir != ".relia/reports" {
		t.Fatalf("ReportDir = %q", options.ReportDir)
	}
}

func TestParseArgsAcceptsCustomPathsAndSaveBaseline(t *testing.T) {
	options, parseErr := ParseArgs([]string{
		"--window", "90d",
		"--format", "json",
		"--baseline", ".relia/baselines/custom.json",
		"--report-dir", ".relia/custom-reports",
		"--save-baseline",
	})
	if parseErr != nil {
		t.Fatalf("ParseArgs returned error: %v", parseErr)
	}
	if options.Window != "90d" || options.BaselinePath != ".relia/baselines/custom.json" || options.ReportDir != ".relia/custom-reports" {
		t.Fatalf("options = %#v", options)
	}
	if !options.FormatExplicit {
		t.Fatal("FormatExplicit = false, want true")
	}
	if !options.SaveBaseline {
		t.Fatal("SaveBaseline = false, want true")
	}
}

func TestParseArgsRejectsUnsupportedFormat(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--format", "text"})
	if parseErr == nil {
		t.Fatal("expected unsupported format error")
	}
	if parseErr.Message != "backtest only supports --format json in this task slice" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsInvalidWindow(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--window", "2w"})
	if parseErr == nil {
		t.Fatal("expected invalid window error")
	}
	if parseErr.Message != "backtest --window must use a day duration such as 180d" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseArgsRejectsInvalidBaselinePath(t *testing.T) {
	_, parseErr := ParseArgs([]string{"--baseline", "../baseline.json"})
	if parseErr == nil {
		t.Fatal("expected invalid baseline path error")
	}
	if parseErr.Message != "backtest --baseline must be a repo-relative path" {
		t.Fatalf("Message = %q", parseErr.Message)
	}
}

func TestParseWindowDaysParsesPositiveDays(t *testing.T) {
	days, parseErr := ParseWindowDays("180d")
	if parseErr != nil {
		t.Fatalf("ParseWindowDays returned error: %v", parseErr)
	}
	if days != 180 {
		t.Fatalf("days = %d, want 180", days)
	}
}
