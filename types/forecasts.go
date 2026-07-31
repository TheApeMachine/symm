package types

import (
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
Forecasts holds calibrated observable predictions derived from typed physical readout.
Strategy consumes these instead of raw projection labels.
*/
type Forecasts struct {
	Source                     string           `json:"source"`
	Symbol                     string           `json:"symbol"`
	At                         time.Time        `json:"at"`
	ObservedInterval           time.Duration    `json:"observedInterval"`
	SourceEpoch                uint64           `json:"sourceEpoch"`
	HorizonEvents              uint64           `json:"horizonEvents"`
	ExpiresEpoch               uint64           `json:"expiresEpoch"`
	Target                     string           `json:"target"`
	ModelVersion               string           `json:"modelVersion"`
	Ready                      bool             `json:"ready"`
	Calibrated                 bool             `json:"calibrated"`
	FrictionReady              bool             `json:"frictionReady"`
	CalibrationSamples         uint64           `json:"calibrationSamples"`
	IncrementalMSE             float64          `json:"incrementalMSE" validate:"finite"`
	IncrementalSkillLowerBound float64          `json:"incrementalSkillLowerBound" validate:"finite"`
	ExpectedReturn             *decimal.Decimal `json:"expectedReturn" validate:"finite"`
	ReferencePrice             *decimal.Decimal `json:"referencePrice" validate:"required"`
	BuyCapacity                *decimal.Decimal `json:"buyCapacity" validate:"required"`
	SellCapacity               *decimal.Decimal `json:"sellCapacity" validate:"required"`
	ExpectedFees               *decimal.Decimal `json:"expectedFees" validate:"finite,nonnegative"`
	ExpectedSpread             *decimal.Decimal `json:"expectedSpread" validate:"finite,nonnegative"`
	ExpectedImpact             *decimal.Decimal `json:"expectedImpact" validate:"finite,nonnegative"`
	ExpectedAdverseSelection   *decimal.Decimal `json:"expectedAdverseSelection" validate:"finite,nonnegative"`
	Uncertainty                float64          `json:"uncertainty" validate:"finite,nonnegative"`
	Confidence                 float64          `json:"confidence" validate:"finite,min=0,max=1"`
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
		forecasts.ExpectedReturn.Float64(),
		forecasts.ExpectedFees.Float64(),
		forecasts.ExpectedSpread.Float64(),
		forecasts.ExpectedImpact.Float64(),
		forecasts.ExpectedAdverseSelection.Float64(),
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

	return forecasts.ExpectedFees.Sign() >= 0 &&
		forecasts.ReferencePrice != nil && forecasts.ReferencePrice.Sign() > 0 &&
		forecasts.BuyCapacity != nil && forecasts.BuyCapacity.Sign() > 0 &&
		forecasts.SellCapacity != nil && forecasts.SellCapacity.Sign() > 0 &&
		forecasts.ExpectedSpread.Sign() >= 0 &&
		forecasts.ExpectedImpact.Sign() >= 0 &&
		forecasts.ExpectedAdverseSelection.Sign() >= 0 &&
		forecasts.Uncertainty >= 0 &&
		forecasts.Confidence >= 0 &&
		forecasts.Confidence <= 1
}

/*
ExecutableReturn subtracts every modeled execution friction 
from the expected market return.
*/
func (forecasts Forecasts) ExecutableReturn() *decimal.Decimal {
	return forecasts.ExpectedReturn.
		Sub(forecasts.ExpectedFees).
		Sub(forecasts.ExpectedSpread).
		Sub(forecasts.ExpectedImpact).
		Sub(forecasts.ExpectedAdverseSelection)
}
