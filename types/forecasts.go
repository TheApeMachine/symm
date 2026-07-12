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
	Source                   string        `json:"source"`
	Symbol                   string        `json:"symbol"`
	At                       time.Time     `json:"at"`
	ObservedInterval         time.Duration `json:"observedInterval"`
	SourceEpoch              uint64        `json:"sourceEpoch"`
	HorizonEvents            uint64        `json:"horizonEvents"`
	ExpiresEpoch             uint64        `json:"expiresEpoch"`
	Target                   string        `json:"target"`
	ModelVersion             string        `json:"modelVersion"`
	Ready                    bool          `json:"ready"`
	Calibrated               bool          `json:"calibrated"`
	FrictionReady            bool          `json:"frictionReady"`
	CalibrationSamples       uint64        `json:"calibrationSamples"`
	IncrementalMSE           float64       `json:"incrementalMSE"`
	IncrementalMSELowerBound float64       `json:"incrementalMSELowerBound"`
	ExpectedReturn           float64       `json:"expectedReturn"`
	ExpectedFees             float64       `json:"expectedFees"`
	ExpectedSpread           float64       `json:"expectedSpread"`
	ExpectedImpact           float64       `json:"expectedImpact"`
	ExpectedAdverseSelection float64       `json:"expectedAdverseSelection"`
	Uncertainty              float64       `json:"uncertainty"`
	Confidence               float64       `json:"confidence"`
}

/*
Eligible reports whether the forecast carries the calibration and provenance
required by strategy. It does not infer missing metadata or thresholds.
*/
func (forecasts Forecasts) Eligible() bool {
	if !forecasts.Ready || forecasts.Source == "" || forecasts.Symbol == "" {
		return false
	}

	if forecasts.Target == "" || forecasts.ModelVersion == "" {
		return false
	}

	if forecasts.At.IsZero() || forecasts.HorizonEvents == 0 ||
		forecasts.ExpiresEpoch <= forecasts.SourceEpoch {
		return false
	}

	values := []float64{
		forecasts.ExpectedReturn,
		forecasts.ExpectedFees,
		forecasts.ExpectedSpread,
		forecasts.ExpectedImpact,
		forecasts.ExpectedAdverseSelection,
		forecasts.Uncertainty,
		forecasts.Confidence,
		forecasts.IncrementalMSE,
		forecasts.IncrementalMSELowerBound,
	}

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return forecasts.ExpectedFees >= 0 &&
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
