package types

import (
	"math"
	"time"
)

/*
Forecasts holds calibrated observable predictions derived from typed physical readout.
Strategy consumes these instead of raw projection labels.
*/
type Forecasts struct {
	Source                     string        `json:"source"`
	Symbol                     string        `json:"symbol"`
	At                         time.Time     `json:"at"`
	ObservedInterval           time.Duration `json:"observedInterval"`
	SourceEpoch                uint64        `json:"sourceEpoch"`
	HorizonEvents              uint64        `json:"horizonEvents"`
	ExpiresEpoch               uint64        `json:"expiresEpoch"`
	Target                     string        `json:"target"`
	ModelVersion               string        `json:"modelVersion"`
	Ready                      bool          `json:"ready"`
	Calibrated                 bool          `json:"calibrated"`
	FrictionReady              bool          `json:"frictionReady"`
	CalibrationSamples         uint64        `json:"calibrationSamples"`
	IncrementalMSE             float64       `json:"incrementalMSE" validate:"finite"`
	IncrementalSkillLowerBound float64       `json:"incrementalSkillLowerBound" validate:"finite"`
	ExpectedReturn             float64       `json:"expectedReturn" validate:"finite"`
	ReferencePrice             float64       `json:"referencePrice" validate:"finite"`
	BuyCapacity                float64       `json:"buyCapacity" validate:"finite"`
	SellCapacity               float64       `json:"sellCapacity" validate:"finite"`
	ExpectedFees               float64       `json:"expectedFees" validate:"finite,nonnegative"`
	ExpectedSpread             float64       `json:"expectedSpread" validate:"finite,nonnegative"`
	ExpectedImpact             float64       `json:"expectedImpact" validate:"finite,nonnegative"`
	ExpectedAdverseSelection   float64       `json:"expectedAdverseSelection" validate:"finite,nonnegative"`
	Uncertainty                float64       `json:"uncertainty" validate:"finite,nonnegative"`
	Confidence                 float64       `json:"confidence" validate:"finite,min=0,max=1"`
}

/*
Eligible reports whether the forecast carries the calibration and provenance
required by strategy. It does not infer missing metadata or thresholds.
*/
func (forecasts Forecasts) Eligible() bool {
	if !forecasts.Ready || !forecasts.Calibrated || !forecasts.FrictionReady ||
		forecasts.Source == "" || forecasts.Symbol == "" {
		return false
	}

	if forecasts.Target == "" || forecasts.ModelVersion == "" {
		return false
	}

	if forecasts.At.IsZero() || forecasts.HorizonEvents == 0 ||
		forecasts.ExpiresEpoch <= forecasts.SourceEpoch {
		return false
	}

	// The one-sided Student-t skill bound requires at least two resolved
	// strict-prior forecasts. A label alone cannot make a forecast calibrated.
	if forecasts.CalibrationSamples < 2 || forecasts.IncrementalSkillLowerBound <= 0 {
		return false
	}

	values := []float64{
		forecasts.ExpectedReturn,
		forecasts.ReferencePrice,
		forecasts.BuyCapacity,
		forecasts.SellCapacity,
		forecasts.ExpectedFees,
		forecasts.ExpectedSpread,
		forecasts.ExpectedImpact,
		forecasts.ExpectedAdverseSelection,
		forecasts.Uncertainty,
		forecasts.Confidence,
		forecasts.IncrementalMSE,
		forecasts.IncrementalSkillLowerBound,
	}

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return forecasts.ExpectedFees >= 0 &&
		forecasts.ReferencePrice > 0 &&
		forecasts.BuyCapacity > 0 &&
		forecasts.SellCapacity > 0 &&
		forecasts.ExpectedSpread >= 0 &&
		forecasts.ExpectedImpact >= 0 &&
		forecasts.ExpectedAdverseSelection >= 0 &&
		forecasts.Uncertainty >= 0 &&
		forecasts.Confidence >= 0 &&
		forecasts.Confidence <= 1
}

/*
ExecutableReturn subtracts every modeled execution friction from the expected
market return.
*/
func (forecasts Forecasts) ExecutableReturn() float64 {
	return forecasts.ExpectedReturn -
		forecasts.ExpectedFees -
		forecasts.ExpectedSpread -
		forecasts.ExpectedImpact -
		forecasts.ExpectedAdverseSelection
}
