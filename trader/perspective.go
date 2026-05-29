package trader

import (
	"errors"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
Perspective is a view on the current market conditions that the trader uses
to make trading decisions, and short-horizon predictions on the future market.
The Perspective, early on, may look at multiple regime candidates, and it is
ready for use once any of these regimes finds enough structural support.

Measurements in one bucket are deliberately ephemeral. The prior implementation
kept appending forever, so a stale high-confidence microstructure reading could
continue to authorize fresh predictions long after the book/trade state had
changed. The bucket now keeps only a short TTL-limited, capped window.
*/
type Perspective struct {
	Ready        bool
	measurements []engine.Measurement
	observedAt   []time.Time
	regimes      map[string]map[string]*numeric.Derived
}

/*
NewPerspective creates a new Perspective from a slice of measurements.
*/
func NewPerspective(measurements []engine.Measurement) *Perspective {
	perspective := &Perspective{
		measurements: make([]engine.Measurement, 0, len(measurements)),
		observedAt:   make([]time.Time, 0, len(measurements)),
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
	now := time.Now()
	perspective.pruneMeasurements(now)
	perspective.measurements = append(perspective.measurements, measurement)
	perspective.observedAt = append(perspective.observedAt, now)
	perspective.pruneMeasurements(now)

	// Ensure there is a valid regime to track.
	if measurement.Regime == "" {
		return
	}

	for _, pair := range measurement.Pairs {
		// Initialize the inner map if it doesn't exist.
		if _, ok := perspective.regimes[pair.Wsname]; !ok {
			perspective.regimes[pair.Wsname] = make(map[string]*numeric.Derived)
		}

		// Initialize the Derived metric tracker for this regime.
		if _, ok := perspective.regimes[pair.Wsname][measurement.Regime]; !ok {
			perspective.regimes[pair.Wsname][measurement.Regime] = numeric.NewDerived(
				numeric.WithDynamics(adaptive.NewEMA(measurement.Confidence)),
			)
		}

		if _, err := perspective.regimes[pair.Wsname][measurement.Regime].Push(
			measurement.Confidence,
		); err != nil {
			errnie.Error(err)
			return
		}
	}
}

func (perspective *Perspective) pruneMeasurements(now time.Time) {
	if perspective == nil || len(perspective.measurements) == 0 {
		return
	}

	if len(perspective.observedAt) != len(perspective.measurements) {
		// Defensive recovery for old tests or hand-built Perspective values.
		perspective.observedAt = make([]time.Time, len(perspective.measurements))

		for index := range perspective.observedAt {
			perspective.observedAt[index] = now
		}
	}

	ttl := config.System.PerspectiveTTL
	write := 0

	for index, measurement := range perspective.measurements {
		observedAt := perspective.observedAt[index]

		if ttl > 0 && !observedAt.IsZero() && observedAt.Before(now.Add(-ttl)) {
			continue
		}

		perspective.measurements[write] = measurement
		perspective.observedAt[write] = observedAt
		write++
	}

	perspective.measurements = perspective.measurements[:write]
	perspective.observedAt = perspective.observedAt[:write]

	limit := config.System.MaxPerspectiveMeasurements

	if limit <= 0 {
		limit = 256
	}

	if len(perspective.measurements) <= limit {
		return
	}

	drop := len(perspective.measurements) - limit
	perspective.measurements = append([]engine.Measurement(nil), perspective.measurements[drop:]...)
	perspective.observedAt = append([]time.Time(nil), perspective.observedAt[drop:]...)
}

/*
Predict makes a short-horizon prediction for this bucket's (symbol, type).
Predictions are always produced for non-empty buckets — the spec calls for
predictions on every batch, not only on entry. Trade selectivity lives
downstream in tryEnter's edge gate, not here.
*/
func (perspective *Perspective) Predict(kind engine.PerspectiveType) (engine.Prediction, error) {
	perspective.pruneMeasurements(time.Now())

	if len(perspective.measurements) == 0 {
		return engine.Prediction{}, ErrNotReady
	}

	jointConfidence, _ := engine.FuseMeasurements(perspective.measurements)

	if jointConfidence <= 0 {
		return engine.Prediction{}, ErrNotReady
	}

	perspective.Ready = true

	now := time.Now()

	enginePerspective := engine.Perspective{
		Type:         kind,
		Measurements: append([]engine.Measurement(nil), perspective.measurements...),
	}

	runway := runwayForPerspective(enginePerspective)

	prediction := engine.Prediction{
		Type:           engine.PredictionTypePump,
		Perspective:    enginePerspective,
		Confidence:     jointConfidence,
		Direction:      predictionDirection(enginePerspective),
		Runway:         runway,
		DueAt:          now.Add(runway),
		PredictedAt:    now,
		ExpectedReturn: jointConfidence,
	}

	return prediction, nil
}

var ErrNotReady = errors.New("perspective not ready")
