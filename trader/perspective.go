package trader

import (
	"errors"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
Perspective is a view on the current market conditions that the trader uses
to make trading decisions, and short-horizon predictions on the future market.
The Perspective, early on, may look at multiple regime candicates, and it is
ready for use once any of these regimes finds enough structural support.
*/
type Perspective struct {
	Ready        bool
	measurements []engine.Measurement
	regimes      map[string]map[string]*numeric.Derived
}

/*
NewPerspective creates a new Perspective from a slice of measurements.
*/
func NewPerspective(measurements []engine.Measurement) *Perspective {
	perspective := &Perspective{
		measurements: make([]engine.Measurement, 0, len(measurements)),
		regimes:      make(map[string]map[string]*numeric.Derived),
	}

	for _, measurement := range measurements {
		perspective.AddMeasurement(measurement)
	}

	return perspective
}

/*
AddMeasurement adds a measurement to the Perspective.
*/
func (perspective *Perspective) AddMeasurement(measurement engine.Measurement) {
	perspective.measurements = append(perspective.measurements, measurement)

	// Ensure there is a valid regime to track
	if measurement.Regime == "" {
		return
	}

	for _, pair := range measurement.Pairs {
		// Initialize the inner map if it doesn't exist
		if _, ok := perspective.regimes[pair.Wsname]; !ok {
			perspective.regimes[pair.Wsname] = make(map[string]*numeric.Derived)
		}

		// Initialize the Derived metric tracker for this regime
		if _, ok := perspective.regimes[pair.Wsname][measurement.Regime]; !ok {
			perspective.regimes[pair.Wsname][measurement.Regime] = numeric.NewDerived(
				numeric.WithDynamics(adaptive.NewEMA(measurement.Confidence)),
			)
		}

		// Push the values into the tracker (including the first measurement)
		if _, err := perspective.regimes[pair.Wsname][measurement.Regime].Push(
			measurement.Confidence,
			measurement.Last,
			measurement.Bid,
			measurement.Ask,
		); err != nil {
			errnie.Error(err)
			return
		}
	}
}

/*
Predict makes a short-horizon prediction on the future market.
*/
func (perspective *Perspective) Predict() (engine.Prediction, error) {
	var bestDerived *numeric.Derived
	var maxConfidence float64

	// 1. Scan your structured regimes to find the one with the highest confidence
	for _, regimes := range perspective.regimes {
		for _, derived := range regimes {
			// Assuming numeric.Derived exposes the current smoothed confidence value
			currentConfidence := derived.Value()

			if currentConfidence > maxConfidence {
				maxConfidence = currentConfidence
				bestDerived = derived
			}
		}
	}

	// 2. Check if the leading candidate has enough "structural support"
	const structuralSupportThreshold = 0.75 // Define based on your risk tolerance
	if maxConfidence < structuralSupportThreshold || bestDerived == nil {
		return engine.Prediction{}, ErrNotReady
	}

	perspective.Ready = true

	now := time.Now()
	kind := engine.PerspectiveMicrostructure

	if len(perspective.measurements) > 0 {
		kind = perspectiveType(perspective.measurements[len(perspective.measurements)-1])
	}

	enginePerspective := engine.Perspective{
		Type:         kind,
		Measurements: append([]engine.Measurement(nil), perspective.measurements...),
	}

	runway := runwayForPerspective(enginePerspective)

	prediction := engine.Prediction{
		Type: engine.PredictionTypePump,
		Perspective: engine.Perspective{
			Type:         kind,
			Measurements: enginePerspective.Measurements,
		},
		Confidence:     maxConfidence,
		Direction:      predictionDirection(enginePerspective),
		Runway:         runway,
		DueAt:          now.Add(runway),
		PredictedAt:    now,
		ExpectedReturn: maxConfidence,
	}

	return prediction, nil
}

var ErrNotReady = errors.New("perspective not ready")
