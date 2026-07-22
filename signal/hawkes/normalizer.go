package hawkes

import (
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/types"
)

/*
normalizer owns dimensionally compatible Hawkes normalization so Evidence only
maps estimator state onto measurement identities and intervals.
*/
type normalizer struct{}

/*
value normalizes a quantity only when a compatible empirical reference exists.
*/
func (normalization normalizer) value(
	outcome excitation.Outcome,
	metric types.MetricType,
	side types.MeasurementSide,
	unit types.MeasurementUnit,
	raw float64,
) *float64 {
	if unit == types.UnitDimensionless {
		return types.NormalizeFinite(raw)
	}

	if unit != types.UnitEventsPerSecond {
		return nil
	}

	switch metric {
	case types.MetricArrivalRate:
		return normalization.arrival(side, outcome, raw)
	case types.MetricConditionalIntensity:
		return normalization.intensity(side, outcome, raw)
	case types.MetricBaselineIntensity:
		return types.NormalizeFinite(raw)
	default:
		return nil
	}
}

/*
arrival uses the marked process baseline before a fit and the fitted baseline
afterward, preserving the readiness distinction in the normalized value.
*/
func (normalization normalizer) arrival(
	side types.MeasurementSide,
	outcome excitation.Outcome,
	raw float64,
) *float64 {
	if !outcome.Readiness.HawkesFit {
		total := outcome.BuyArrivalRate + outcome.SellArrivalRate

		if total <= 0 {
			return nil
		}

		return types.NormalizeDeviation(raw, total/2)
	}

	return normalization.intensity(side, outcome, raw)
}

/*
intensity normalizes conditional intensity against its same-side fitted
baseline so excitation remains directional.
*/
func (normalization normalizer) intensity(
	side types.MeasurementSide,
	outcome excitation.Outcome,
	raw float64,
) *float64 {
	switch side {
	case types.SideBuy:
		return types.NormalizeDeviation(raw, outcome.Fit.MuX)
	case types.SideSell:
		return types.NormalizeDeviation(raw, outcome.Fit.MuY)
	default:
		return nil
	}
}
