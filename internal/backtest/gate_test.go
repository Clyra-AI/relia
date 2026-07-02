package backtest

import (
	"testing"

	"github.com/Clyra-AI/relia/internal/yamlmini"
)

func TestBuildGateDefaultsToOff(t *testing.T) {
	gate := BuildGate(yamlmini.Document{}, 0.75)

	if gate.Enabled || gate.Status != "off" || gate.Ref != "relia.yaml" {
		t.Fatalf("gate = %#v, want disabled off gate at default ref", gate)
	}
	if gate.Threshold != nil {
		t.Fatalf("threshold = %v, want nil when gate is off", *gate.Threshold)
	}
}

func TestBuildGateFailsWhenHeadlineExceedsConfiguredThreshold(t *testing.T) {
	document, err := yamlmini.ParseDocument("gate:\n  enabled: true\n  max_error_recurrence_rate: 0.25\n")
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	gate := BuildGate(document, 0.5)

	if !gate.Enabled || gate.Status != "fail" || gate.Ref != "relia.yaml:3" {
		t.Fatalf("gate = %#v, want failing enabled gate at threshold ref", gate)
	}
	if got := GateThresholdValue(gate); got != 0.25 {
		t.Fatalf("GateThresholdValue = %v, want 0.25", got)
	}
}

func TestBuildGateIgnoresInvalidThreshold(t *testing.T) {
	document, err := yamlmini.ParseDocument("gate:\n  enabled: true\n  max_error_recurrence_rate: 2\n")
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	gate := BuildGate(document, 0.5)

	if !gate.Enabled || gate.Status != "pass" || gate.Ref != "relia.yaml:2" {
		t.Fatalf("gate = %#v, want default threshold and enabled ref", gate)
	}
	if got := GateThresholdValue(gate); got != 1.0 {
		t.Fatalf("GateThresholdValue = %v, want 1.0", got)
	}
}

func TestGateThresholdValueReturnsZeroForMissingThreshold(t *testing.T) {
	if got := GateThresholdValue(GateResult{}); got != 0 {
		t.Fatalf("GateThresholdValue = %v, want 0", got)
	}
}
