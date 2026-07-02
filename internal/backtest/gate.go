package backtest

import (
	"strconv"

	configdoc "github.com/Clyra-AI/relia/internal/config"
	"github.com/Clyra-AI/relia/internal/yamlmini"
)

func BuildGate(config yamlmini.Document, headlineERR float64) GateResult {
	enabled := false
	ref := configdoc.DefaultFile
	if scalar, ok := config.Scalars["gate.enabled"]; ok {
		enabled = scalar.Value == "true"
		ref = configdoc.Ref(configdoc.DefaultFile, scalar)
	}
	if !enabled {
		return GateResult{
			Enabled: false,
			Status:  "off",
			Reason:  "Recurrence gate is available but disabled by default for advisory-only MVP behavior.",
			Ref:     ref,
		}
	}
	threshold := 1.0
	if scalar, ok := config.Scalars["gate.max_error_recurrence_rate"]; ok {
		if parsed, err := strconv.ParseFloat(scalar.Value, 64); err == nil && parsed >= 0 && parsed <= 1 {
			threshold = parsed
			ref = configdoc.Ref(configdoc.DefaultFile, scalar)
		}
	}
	status := "pass"
	reason := "Headline ERR is within the configured recurrence gate."
	if headlineERR > threshold {
		status = "fail"
		reason = "Headline ERR exceeds the configured recurrence gate."
	}
	return GateResult{
		Enabled:   true,
		Status:    status,
		Threshold: &threshold,
		Reason:    reason,
		Ref:       ref,
	}
}

func GateThresholdValue(gate GateResult) float64 {
	if gate.Threshold == nil {
		return 0
	}
	return *gate.Threshold
}
