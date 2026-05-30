package price

import (
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/market/perspectives"
)

// PerspectiveRecord is the result of recording one perspective-level forecast.
// Fresh is true whenever a usable forecast was produced; it is only false for an
// empty record (no priceable anchor or non-positive confidence). It no longer
// gates on an in-flight forward-return measurement -- entry eligibility is
// decided from Tradable and the entry economics, not from settlement state.
type PerspectiveRecord struct {
	PredictedReturn float64
	PredictedAt     time.Time
	DueAt           time.Time
	Runway          time.Duration
	Fresh           bool
	Tradable        bool
	Explorable      bool // priced & confident, but no demonstrated edge yet -- eligible for exploration
	SampleCount     int  // settled forward-return samples in this (source, regime) bucket
	Confidence      float64
	Source          string
	Sources         []string
	Contributions   map[string]float64
}

func (prediction *Prediction) RecordPerspective(
	symbol string,
	perspective perspectives.Perspective,
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

	// Forward-return estimate is the data-shrunk realized mean for this
	// (source, regime) -- no minimum-sample or significance cliff (see
	// ReturnModel.ExpectedReturn). Confidence is not folded in here; it scales
	// position size downstream via Kelly. A (source, regime) with no
	// demonstrated edge yields ~0, which the entry economics treat as "no trade"
	// without an explicit gate. This is the dynamic, per-regime signal
	// selection: a signal only earns size where its own settled forward returns
	// show an edge in the current regime.
	predictedReturn, reliability := prediction.returnModel.ExpectedReturn(source, regime)
	tradable := reliability > 0

	// A priced, confident perspective with no demonstrated edge yet is not
	// tradable on the disciplined gate, but it IS explorable: exploration can
	// take a small position to gather the forward returns this bucket needs to
	// learn its edge. sampleCount lets the trader decide cold vs warm. Both
	// anchor-priced and confidence>0 are already guaranteed above, so any record
	// that reaches here is explorable.
	sampleCount := prediction.returnModel.SampleCount(source, regime)

	runway := perspectiveRunway(perspective)
	predictedAt := now
	dueAt := now.Add(runway)

	bySource := prediction.open[symbol]

	if bySource == nil {
		bySource = make(map[string]openPrediction)
		prediction.open[symbol] = bySource
	}

	// Maintain exactly one in-flight forward-return measurement per (symbol,
	// source). It settles on its own schedule in settleDue to feed the return
	// model; refinements must NOT reset its clock, or it would never reach its
	// due time and the model would starve of training samples.
	//
	// Entry eligibility is a SEPARATE question, answered by the forecast just
	// computed off the *current* perspective (predictedReturn/tradable above).
	// A measurement already being in flight no longer blocks a fresh entry --
	// conflating the two was the trade-starvation bug: every perspective after
	// the first logged "open_prediction_pending" and skipped for the entire
	// runway window, so the trader could enter at most once per (symbol,
	// source) per runway and only if the model happened to be ready on that
	// exact tick. The forecast is recomputed from current confidence and a
	// current anchor, so there is no stale-anchor risk; re-entering a symbol we
	// already hold is prevented downstream by holdsSymbol.
	if _, ok := bySource[source]; !ok {
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

		// Broadcast the forecast only when a new measurement opens, not on every
		// refinement, so the UI/prediction chart keeps one point per forecast.
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
	}

	return PerspectiveRecord{
		PredictedReturn: predictedReturn,
		PredictedAt:     predictedAt,
		DueAt:           dueAt,
		Runway:          runway,
		Fresh:           true,
		Tradable:        tradable,
		Explorable:      true,
		SampleCount:     sampleCount,
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
