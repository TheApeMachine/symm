package logic

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

func (decision *Decision) stale(
	reference time.Time,
	measurements map[types.SourceType]*types.Measurement,
) bool {
	if decision.maxAge <= 0 || reference.IsZero() {
		return false
	}

	for _, measurement := range measurements {
		if measurement == nil || measurement.At.IsZero() {
			return true
		}

		age := reference.Sub(measurement.At)
		if age < 0 {
			age = -age
		}

		if age > decision.maxAge {
			return true
		}
	}

	return false
}

func (decision *Decision) cascadeSource(source types.SourceType) bool {
	return source == types.SourceManifold ||
		source == types.SourceResonance ||
		source == types.SourceCausal
}

func (decision *Decision) validate(
	measurement *types.Measurement,
) error {
	values := []float64{
		measurement.EntryBaseline,
		measurement.ExitBaseline,
	}

	for _, value := range values {
		if !finite(value) {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"decision: measurement evidence must be finite",
				nil,
			))
		}
	}

	for _, row := range measurement.Categories {
		if !finite(row.Confidence) ||
			!finite(row.Strength) ||
			!finite(row.Surprisal) {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"decision: measurement category evidence must be finite",
				nil,
			))
		}

		if row.Confidence < 0 || row.Confidence > 1 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"decision: measurement category confidence must be a probability",
				nil,
			))
		}
	}

	for _, value := range measurement.Metrics {
		if !finite(value) {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"decision: measurement metrics must be finite",
				nil,
			))
		}
	}

	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
