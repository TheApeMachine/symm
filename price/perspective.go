package price

import (
	"math"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

func (prediction *Prediction) RecordPerspective(
	symbol string,
	perspective engine.Perspective,
	now time.Time,
) float64 {
	prediction.stateMu.Lock()
	defer prediction.stateMu.Unlock()

	anchorMeasurement, ok := perspectiveAnchorMeasurement(symbol, perspective)

	if !ok {
		return 0
	}

	source := engine.PerspectiveSource(perspective.Type)
	confidence, _ := engine.FuseMeasurements(perspective.Measurements)
	key := predictionSeriesKey{source: source, symbol: symbol}
	scale := prediction.returnScale(source, symbol)

	if prediction.returnSeen[key] {
		if scale == 0 {
			return 0
		}
	} else if scale <= 0 {
		scale = impliedReturnScale(anchorMeasurement)

		if scale <= 0 {
			return 0
		}
	}

	predictedReturn := confidence * scale
	runway := perspectiveRunway(perspective)

	bySource := prediction.open[symbol]

	if bySource == nil {
		bySource = make(map[string]openPrediction)
		prediction.open[symbol] = bySource
	}

	bySource[source] = openPrediction{
		perspective:     perspective,
		measurement:     anchorMeasurement,
		source:          source,
		sources:         perspectiveSources(perspective),
		predictedReturn: predictedReturn,
		confidence:      confidence,
		anchorPrice:     anchorPrice(anchorMeasurement),
		direction:       perspectiveDirection(perspective),
		runway:          runway,
		dueAt:           now.Add(runway),
		predictedAt:     now,
	}

	prediction.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":     "prediction",
		"source":    source,
		"sources":   perspectiveSources(perspective),
		"symbol":    symbol,
		"value":     predictedReturn,
		"ts":        now.UTC().Format(time.RFC3339Nano),
		"due_at":    now.Add(runway).UTC().Format(time.RFC3339Nano),
		"runway_ms": runway.Milliseconds(),
	}})

	return predictedReturn
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

func impliedReturnScale(measurement engine.Measurement) float64 {
	anchor := anchorPrice(measurement)

	if anchor <= 0 {
		return 0
	}

	spreadReturn := 0.0

	if measurement.Bid > 0 && measurement.Ask > 0 {
		spreadReturn = (measurement.Ask - measurement.Bid) / anchor
	}

	feePct := config.System.TakerFeePct * 2

	if config.System.UseMakerEntries {
		feePct = config.System.MakerFeePct + config.System.TakerFeePct
	}

	feeReturn := feePct / 100

	if spreadReturn <= 0 {
		return 0
	}

	implied := spreadReturn/2 + feeReturn + spreadReturn*config.System.ForecastSpreadMultiple

	return math.Max(implied, feeReturn)
}
