package backtest

import (
	"encoding/json"
	"time"

	ingestdoc "github.com/Clyra-AI/relia/internal/ingest"
)

type Experience struct {
	Record     ingestdoc.Record
	RecordedAt time.Time
	SourcePath string
	SourceLine int
}

type RecurrenceReport struct {
	ObjectType           string               `json:"object_type"`
	SchemaVersion        string               `json:"schema_version"`
	ReportID             string               `json:"report_id"`
	SourceArtifacts      []string             `json:"source_artifacts"`
	Window               RecurrenceWindow     `json:"window"`
	Summary              RecurrenceSummary    `json:"summary"`
	Metrics              RecurrenceMetrics    `json:"metrics"`
	HeadlineERR          float64              `json:"headline_err"`
	ConfirmedRecurrences []RecurrencePair     `json:"confirmed_recurrences"`
	PossibleRecurrences  []RecurrencePair     `json:"possible_recurrences"`
	TopRepeatedMistakes  []TopRepeatedMistake `json:"top_repeated_mistakes"`
	FlakeDiscounts       []FlakeDiscount      `json:"flake_discounts"`
	AttributionUncertain []Uncertain          `json:"attribution_uncertain"`
	Baseline             BaselineComparison   `json:"baseline"`
	Gate                 GateResult           `json:"gate"`
	Citations            []Citation           `json:"citations"`
	Diagnostics          []ReportDiagnostic   `json:"diagnostics"`
	OperatorFeedback     OperatorFeedback     `json:"operator_feedback"`
	Badge                ReportBadge          `json:"badge"`
	Metadata             map[string]any       `json:"metadata"`
}

type RecurrenceWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type RecurrenceMetrics struct {
	PRsAnalyzed                int            `json:"prs_analyzed"`
	AgentAttributedPRs         int            `json:"agent_attributed_prs"`
	AgentAttributedExperiences int            `json:"agent_attributed_experiences"`
	AgentFailuresByOutcomeKind map[string]int `json:"agent_failures_by_outcome_kind"`
	ErrorRecurrenceRate        float64        `json:"error_recurrence_rate"`
	ConfirmedRecurrences       int            `json:"confirmed_recurrences"`
	PossibleRecurrences        int            `json:"possible_recurrences"`
	FlakeDiscountedCount       int            `json:"flake_discounted_count"`
	AttributionUncertainCount  int            `json:"attribution_uncertain_count"`
}

type RecurrenceSummary struct {
	ExperienceCount           int     `json:"experience_count"`
	WindowExperienceCount     int     `json:"window_experience_count"`
	AgentFailureDenominator   int     `json:"agent_failure_denominator"`
	ConfirmedRecurrenceCount  int     `json:"confirmed_recurrence_count"`
	PossibleRecurrenceCount   int     `json:"possible_recurrence_count"`
	HeadlineERR               float64 `json:"headline_err"`
	HeadlineERRPercent        string  `json:"headline_err_percent"`
	FlakeDiscountedCount      int     `json:"flake_discounted_count"`
	AttributionUncertainCount int     `json:"attribution_uncertain_count"`
	HumanFailureExcludedCount int     `json:"human_failure_excluded_count"`
	NonFailureOutcomeCount    int     `json:"non_failure_outcome_count"`
}

type RecurrencePair struct {
	CurrentExperienceID string   `json:"current_experience_id"`
	PriorExperienceID   string   `json:"prior_experience_id"`
	CurrentPR           int      `json:"current_pr"`
	PriorPR             int      `json:"prior_pr"`
	CurrentURL          string   `json:"current_url"`
	PriorURL            string   `json:"prior_url"`
	SignatureID         string   `json:"signature_id"`
	MatchedSignatureID  string   `json:"matched_signature_id,omitempty"`
	Confidence          string   `json:"confidence"`
	Reason              string   `json:"reason"`
	Refs                []string `json:"refs"`
}

type TopRepeatedMistake struct {
	Rank          int      `json:"rank"`
	SignatureID   string   `json:"signature_id"`
	RepeatCount   int      `json:"repeat_count"`
	PRs           []int    `json:"prs"`
	URLs          []string `json:"urls"`
	ExperienceIDs []string `json:"experience_ids"`
	Refs          []string `json:"refs"`
}

type FlakeDiscount struct {
	ExperienceID    string   `json:"experience_id"`
	PR              int      `json:"pr"`
	SignatureID     string   `json:"signature_id"`
	FlakeDiscount   float64  `json:"flake_discount"`
	SupportingPRs   []int    `json:"supporting_prs"`
	SupportingRefs  []string `json:"supporting_refs"`
	Reason          string   `json:"reason"`
	ExcludedFromERR bool     `json:"excluded_from_err"`
}

type Uncertain struct {
	ExperienceID          string  `json:"experience_id"`
	PR                    int     `json:"pr"`
	OutcomeKind           string  `json:"outcome_kind"`
	AttributionMethod     string  `json:"attribution_method"`
	AttributionConfidence float64 `json:"attribution_confidence"`
	ExcludedFromERR       bool    `json:"excluded_from_err"`
	Ref                   string  `json:"ref"`
	Reason                string  `json:"reason"`
}

type BaselineComparison struct {
	Status      string  `json:"status"`
	Path        string  `json:"path"`
	HeadlineERR float64 `json:"headline_err,omitempty"`
	Delta       float64 `json:"delta,omitempty"`
	Stale       bool    `json:"stale"`
	Reason      string  `json:"reason"`
}

func (comparison BaselineComparison) MarshalJSON() ([]byte, error) {
	type baselineComparisonJSON struct {
		Status      string   `json:"status"`
		Path        string   `json:"path"`
		HeadlineERR *float64 `json:"headline_err,omitempty"`
		Delta       *float64 `json:"delta,omitempty"`
		Stale       bool     `json:"stale"`
		Reason      string   `json:"reason"`
	}
	payload := baselineComparisonJSON{
		Status: comparison.Status,
		Path:   comparison.Path,
		Stale:  comparison.Stale,
		Reason: comparison.Reason,
	}
	if comparison.Status != "missing" {
		headlineERR := comparison.HeadlineERR
		delta := comparison.Delta
		payload.HeadlineERR = &headlineERR
		payload.Delta = &delta
	}
	return json.Marshal(payload)
}

type GateResult struct {
	Enabled   bool     `json:"enabled"`
	Status    string   `json:"status"`
	Threshold *float64 `json:"threshold,omitempty"`
	Reason    string   `json:"reason"`
	Ref       string   `json:"ref"`
}

type Citation struct {
	PR           int    `json:"pr"`
	URL          string `json:"url"`
	ExperienceID string `json:"experience_id"`
}

type ReportDiagnostic struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Ref     string `json:"ref"`
}

type OperatorFeedback struct {
	Summary                  string `json:"summary"`
	ConservativeMatchingNote string `json:"conservative_matching_note"`
	NextCommand              string `json:"next_command"`
}

type ReportBadge struct {
	Label          string `json:"label"`
	Message        string `json:"message"`
	Status         string `json:"status"`
	Stale          bool   `json:"stale"`
	Color          string `json:"color"`
	Reason         string `json:"reason"`
	SourceReportID string `json:"source_report_id"`
}
