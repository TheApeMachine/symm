package price

import (
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

// PerspectiveRecord is the result of recording one perspective-level forecast.
// Fresh is false when an unsettled (symbol, perspective-source) forecast already
// exists; callers should not trade again off that stale anchor.
type PerspectiveRecord struct {
	PredictedReturn float64
	PredictedAt     time.Time
	DueAt           time.Time
	Runway          time.Duration
	Fresh           bool
	Tradable        bool
	Confidence      float64
	Source          string
	Sources         []string
	Contributions   map[string]float64
}

func (prediction *Prediction) RecordPerspective(
	symbol string,
	perspective engine.Perspective,
	now time.Time,
) PerspectiveRecord {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	anchorMeasurement, ok := perspectiveAnchorMeasurement(symbol, perspective)

	if !ok {
		return PerspectiveRecord{}
	}

	source := engine.PerspectiveSource(perspective.Type)
	confidence, _ := engine.FuseMeasurements(perspective.Measurements)

	if confidence <= 0 {
		return PerspectiveRecord{}
	}

	regime := engine.FeedbackRegime(perspective, anchorMeasurement)
	sources := perspectiveSources(perspective)
	contributions := perspectiveContributions(perspective)

	minSamples := configuredForwardMinSamples()

	if anchorMeasurement.Source == "pumpdump" {
		minSamples = configuredPumpForwardMinSamples()
	}

	predictedReturn, tradable := prediction.returnModel.ForecastWithMin(
		source, regime, confidence, minSamples,
	)

	runway := perspectiveRunway(perspective)
	predictedAt := now
	dueAt := now.Add(runway)

	bySource := prediction.open[symbol]

	if bySource == nil {
		bySource = make(map[string]openPrediction)
		prediction.open[symbol] = bySource
	}

	// One open prediction per (symbol, source) at a time; refinements do not
	// reset the clock. Returning Fresh=false prevents the trader from entering
	// on an old forecast whose anchor price/runway belong to a previous book
	// state.
	if existing, ok := bySource[source]; ok {
		return PerspectiveRecord{
			PredictedReturn: existing.predictedReturn,
			PredictedAt:     existing.predictedAt,
			DueAt:           existing.dueAt,
			Runway:          existing.runway,
			Fresh:           false,
			Tradable:        existing.predictedReturn > 0,
			Confidence:      existing.confidence,
			Source:          existing.source,
			Sources:         append([]string(nil), existing.sources...),
			Contributions:   copyContributions(existing.contributions),
		}
	}

	bySource[source] = openPrediction{
		perspective:     perspective,
		measurement:     anchorMeasurement,
		source:          source,
		sources:         sources,
		contributions:   contributions,
		regime:          regime,
		predictedReturn: predictedReturn,
		confidence:      confidence,
		anchorPrice:     anchorPrice(anchorMeasurement),
		direction:       perspectiveDirection(perspective),
		runway:          runway,
		dueAt:           dueAt,
		predictedAt:     predictedAt,
	}

	prediction.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":         "prediction",
		"source":        source,
		"sources":       sources,
		"contributions": contributions,
		"symbol":        symbol,
		"value":         predictedReturn,
		"tradable":      tradable,
		"confidence":    confidence,
		"ts":            predictedAt.UTC().Format(time.RFC3339Nano),
		"due_at":        dueAt.UTC().Format(time.RFC3339Nano),
		"runway_ms":     runway.Milliseconds(),
	}})

	return PerspectiveRecord{
		PredictedReturn: predictedReturn,
		PredictedAt:     predictedAt,
		DueAt:           dueAt,
		Runway:          runway,
		Fresh:           true,
		Tradable:        tradable,
		Confidence:      confidence,
		Source:          source,
		Sources:         sources,
		Contributions:   contributions,
	}
}

func perspectiveAnchorMeasurement(
	symbol string,
	perspective engine.Perspective,
) (engine.Measurement, bool) {
	best := engine.Measurement{}

	for _, measurement := range perspective.Measurements {
		if len(measurement.Pairs) == 0 || measurement.Pairs[0].Wsname != symbol {
			continue
		}

		if anchorPrice(measurement) <= 0 {
			continue
		}

		if measurement.Confidence <= best.Confidence {
			continue
		}

		best = measurement
	}

	return best, best.Confidence > 0
}

func perspectiveSources(perspective engine.Perspective) []string {
	seen := make(map[string]struct{})
	sources := make([]string, 0, len(perspective.Measurements))

	for _, measurement := range perspective.Measurements {
		if measurement.Source == "" {
			continue
		}

		if _, ok := seen[measurement.Source]; ok {
			continue
		}

		seen[measurement.Source] = struct{}{}
		sources = append(sources, measurement.Source)
	}

	return sources
}

func perspectiveContributions(perspective engine.Perspective) map[string]float64 {
	contributions := make(map[string]float64)

	for _, measurement := range perspective.Measurements {
		if measurement.Source == "" || measurement.Confidence <= 0 {
			continue
		}

		if measurement.Confidence > contributions[measurement.Source] {
			contributions[measurement.Source] = measurement.Confidence
		}
	}

	return contributions
}

func copyContributions(source map[string]float64) map[string]float64 {
	if source == nil {
		return nil
	}

	copied := make(map[string]float64, len(source))

	for key, value := range source {
		copied[key] = value
	}

	return copied
}

func perspectiveDirection(perspective engine.Perspective) int {
	score := 0.0

	for _, measurement := range perspective.Measurements {
		score += measurement.Confidence * float64(measurementDirection(measurement))
	}

	if score < 0 {
		return -1
	}

	return 1
}

func perspectiveRunway(perspective engine.Perspective) time.Duration {
	runway := time.Duration(0)

	for _, measurement := range perspective.Measurements {
		candidate := measurementRunway(measurement)

		if candidate > runway {
			runway = candidate
		}
	}

	if runway > 0 {
		return runway
	}

	return config.System.ScalpHoldBeforeExit
}

func anchorPrice(measurement engine.Measurement) float64 {
	if measurement.Last > 0 {
		return measurement.Last
	}

	if measurement.Bid > 0 && measurement.Ask > 0 {
		return (measurement.Bid + measurement.Ask) / 2
	}

	return 0
}
