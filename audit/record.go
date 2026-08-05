package audit

import (
	"fmt"
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

type Category string

const (
	CategoryDecisionContext     Category = "decision_context"
	CategorySignalEvidence      Category = "signal_evidence"
	CategoryForecastIssued      Category = "forecast_issued"
	CategoryForecastOutcome     Category = "forecast_outcome"
	CategoryExecutionLifecycle  Category = "execution_lifecycle"
	CategoryEvidenceUnavailable Category = "evidence_unavailable"
	CategoryModelValidation     Category = "model_validation"
)

/*
Event is one curated analytical fact. The unexported category method closes the
set to the typed events in this package, preventing arbitrary metric maps from
turning the audit file into a second market-history cache.
*/
type Event interface {
	Validate() error
	category() Category
}

type DecisionContext struct {
	DecisionID       string               `json:"decision_id"`
	Symbol           string               `json:"symbol"`
	At               time.Time            `json:"at"`
	Tick             int64                `json:"tick"`
	Action           types.Action         `json:"action"`
	Cause            string               `json:"cause"`
	Reason           string               `json:"reason"`
	Utility          float64              `json:"utility"`
	Alternatives     map[string]float64   `json:"alternatives,omitempty"`
	Opportunity      bool                 `json:"opportunity"`
	AllocationClass  string               `json:"allocation_class,omitempty"`
	AllocationCut    float64              `json:"allocation_haircut"`
	AllocationReason string               `json:"allocation_haircut_reason,omitempty"`
	ProposedQuantity *decimal.Decimal     `json:"proposed_quantity,omitempty"`
	ProposedNotional *decimal.Decimal     `json:"proposed_notional,omitempty"`
	Risk             types.RiskPlan       `json:"risk"`
	Trace            *types.DecisionTrace `json:"trace,omitempty"`
}

func (event DecisionContext) category() Category { return CategoryDecisionContext }

func (event DecisionContext) Validate() error {
	if event.DecisionID == "" || event.Symbol == "" || event.At.IsZero() ||
		event.Tick <= 0 || event.Cause == "" || event.Reason == "" {
		return fmt.Errorf("audit: incomplete decision context")
	}

	if !finite(event.Utility) || !finite(event.AllocationCut) {
		return fmt.Errorf("audit: non-finite decision context")
	}

	return nil
}

type SignalEvidence struct {
	DecisionID string             `json:"decision_id"`
	DecisionAt time.Time          `json:"decision_at"`
	Evidence   *types.Measurement `json:"evidence"`
}

func (event SignalEvidence) category() Category { return CategorySignalEvidence }

func (event SignalEvidence) Validate() error {
	if event.DecisionID == "" || event.DecisionAt.IsZero() || event.Evidence == nil {
		return fmt.Errorf("audit: incomplete signal evidence")
	}

	if event.Evidence.Validity.State != types.ValidityValid ||
		event.Evidence.Source == "" || event.Evidence.Symbol == "" ||
		event.Evidence.At.IsZero() || event.Evidence.At.After(event.DecisionAt) {
		return fmt.Errorf("audit: signal evidence is not causally valid")
	}

	if err := event.Evidence.ValidateStruct(); err != nil {
		return fmt.Errorf("audit: invalid signal evidence: %w", err)
	}

	for _, sample := range event.Evidence.Metrics {
		if !finite(sample.Raw) ||
			(sample.Normalized != nil && !finite(*sample.Normalized)) {
			return fmt.Errorf("audit: non-finite signal metric")
		}
	}

	return nil
}

type ForecastIssued struct {
	DecisionID       string           `json:"decision_id"`
	Symbol           string           `json:"symbol"`
	IssuedAt         time.Time        `json:"issued_at"`
	Source           string           `json:"source"`
	Model            string           `json:"model,omitempty"`
	Epoch            uint64           `json:"epoch"`
	ValidThrough     uint64           `json:"valid_through"`
	HorizonSteps     int              `json:"horizon_steps"`
	CalibrationCount uint64           `json:"calibration_count"`
	ReferencePrice   *decimal.Decimal `json:"reference_price"`
	ExpectedReturn   *decimal.Decimal `json:"expected_return"`
	ExpectedFees     *decimal.Decimal `json:"expected_fees"`
	ExpectedSpread   *decimal.Decimal `json:"expected_spread"`
	ExpectedImpact   *decimal.Decimal `json:"expected_impact"`
	Confidence       float64          `json:"confidence"`
	Uncertainty      float64          `json:"uncertainty"`
}

func (event ForecastIssued) category() Category { return CategoryForecastIssued }

func (event ForecastIssued) Validate() error {
	if event.DecisionID == "" || event.Symbol == "" || event.IssuedAt.IsZero() ||
		event.Source == "" || event.Epoch == 0 || event.ValidThrough == 0 ||
		event.HorizonSteps <= 0 ||
		event.ReferencePrice == nil || event.ReferencePrice.Sign() <= 0 ||
		event.ExpectedReturn == nil || event.ExpectedFees == nil ||
		event.ExpectedSpread == nil || event.ExpectedImpact == nil {
		return fmt.Errorf("audit: incomplete forecast issuance")
	}

	if !finite(event.Confidence) || !finite(event.Uncertainty) ||
		event.Uncertainty < 0 {
		return fmt.Errorf("audit: invalid forecast confidence")
	}

	return nil
}

type EvidenceUnavailable struct {
	DecisionID string                     `json:"decision_id"`
	Symbol     string                     `json:"symbol"`
	At         time.Time                  `json:"at"`
	Source     types.SourceType           `json:"source"`
	State      types.ValidityState        `json:"state,omitempty"`
	Readiness  types.MeasurementReadiness `json:"readiness,omitempty"`
	Reason     string                     `json:"reason"`
}

func (event EvidenceUnavailable) category() Category { return CategoryEvidenceUnavailable }

func (event EvidenceUnavailable) Validate() error {
	if event.DecisionID == "" || event.Symbol == "" || event.At.IsZero() ||
		event.Source == "" || event.Reason == "" {
		return fmt.Errorf("audit: incomplete unavailable evidence")
	}

	return nil
}

type ForecastOutcome struct {
	ResolvedAt time.Time            `json:"resolved_at"`
	Provenance string               `json:"provenance"`
	Episode    types.PassageEpisode `json:"episode"`
}

func (event ForecastOutcome) category() Category { return CategoryForecastOutcome }

func (event ForecastOutcome) Validate() error {
	if event.ResolvedAt.IsZero() || event.Provenance == "" ||
		event.Episode.PositionID == "" || event.Episode.Symbol == "" ||
		event.Episode.Horizon <= 0 {
		return fmt.Errorf("audit: incomplete forecast outcome")
	}

	return nil
}

const ExecutionStopTransition = "stop_transition"

type ExecutionLifecycle struct {
	PositionID  string               `json:"position_id"`
	Symbol      string               `json:"symbol"`
	Kind        string               `json:"kind"`
	Transition  types.StopTransition `json:"transition"`
	StopOrderID string               `json:"stop_order_id,omitempty"`
	ExitAttempt uint64               `json:"exit_attempt,omitempty"`
}

func (event ExecutionLifecycle) category() Category { return CategoryExecutionLifecycle }

func (event ExecutionLifecycle) Validate() error {
	if event.PositionID == "" || event.Symbol == "" ||
		event.Kind != ExecutionStopTransition || event.Transition.At.IsZero() ||
		event.Transition.Reason == "" {
		return fmt.Errorf("audit: incomplete execution lifecycle")
	}

	return nil
}

type ModelValidation struct {
	Component  string    `json:"component"`
	Symbol     string    `json:"symbol,omitempty"`
	At         time.Time `json:"at"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason"`
	Error      string    `json:"error,omitempty"`
	Observed   int       `json:"observed,omitempty"`
	Retained   int       `json:"retained,omitempty"`
	Dropped    int       `json:"dropped,omitempty"`
	Resident   int       `json:"resident,omitempty"`
	BatchStart int       `json:"batch_start,omitempty"`
	BatchSize  int       `json:"batch_size,omitempty"`
	Capacity   int       `json:"capacity,omitempty"`
}

func (event ModelValidation) category() Category { return CategoryModelValidation }

func (event ModelValidation) Validate() error {
	if event.Component == "" || event.At.IsZero() || event.Status == "" ||
		event.Reason == "" {
		return fmt.Errorf("audit: incomplete model validation")
	}

	return nil
}

/*
Record validates and writes one typed analytical event.
*/
func Record(recorder *Recorder, event Event) error {
	if recorder == nil {
		return nil
	}

	if event == nil {
		return fmt.Errorf("audit: event required")
	}

	if err := event.Validate(); err != nil {
		return err
	}

	return recorder.Write(map[string]any{
		"channel": "analysis",
		"type":    event.category(),
		"value":   event,
	})
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
