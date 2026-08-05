package hawkes

import (
	"math"
	"time"

	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/types"
)

/*
normalizer owns the Hawkes-specific scale contracts. Counts are measured
against estimator support, rates against same-process rates, amplitudes against
decay, likelihood improvements per observed event, and kernel time against the
actual observation horizon.
*/
type normalizer struct{}

const (
	hawkesMarkedSides    = 2.0
	hawkesCriticalBranch = 1.0
)

/*
value returns no normalized reading until the evidence required by that
metric's contract exists. Raw values remain on the Measurement either way.
*/
func (normalization normalizer) value(
	outcome excitation.Outcome,
	metric types.MetricType,
	side types.MeasurementSide,
	unit types.MeasurementUnit,
	raw float64,
) *float64 {
	if !finite(raw) {
		return nil
	}

	switch metric {
	case types.MetricEventCount:
		if unit != types.UnitCount {
			return nil
		}

		return normalization.eventCount(outcome, side, raw)
	case types.MetricArrivalRate:
		if unit != types.UnitEventsPerSecond {
			return nil
		}

		return normalization.arrival(outcome, side, raw)
	case types.MetricConditionalIntensity:
		if unit != types.UnitEventsPerSecond || !outcome.Readiness.HawkesFit {
			return nil
		}

		return normalization.conditionalIntensity(outcome, side, raw)
	case types.MetricBaselineIntensity:
		if unit != types.UnitEventsPerSecond || !outcome.Readiness.HawkesFit {
			return nil
		}

		return normalization.baselineShare(outcome, raw)
	case types.MetricExcitationAmplitude:
		if unit != types.UnitEventsPerSecond || !outcome.Readiness.HawkesFit {
			return nil
		}

		if raw < 0 {
			return nil
		}

		return normalization.ratio(raw, outcome.Fit.Beta)
	case types.MetricDecayRate:
		if unit != types.UnitInverseSecond || !outcome.Readiness.HawkesFit {
			return nil
		}

		if raw <= 0 {
			return nil
		}

		return normalization.ratio(raw, outcome.Fit.MuX+outcome.Fit.MuY)
	case types.MetricKernelMemory:
		if unit != types.UnitSecond || !outcome.Readiness.HawkesFit {
			return nil
		}

		if raw <= 0 {
			return nil
		}

		return normalization.ratio(raw, observationHorizon(outcome).Seconds())
	case types.MetricHawkesPoissonDelta, types.MetricCrossSelfDelta:
		if unit != types.UnitNat || !outcome.Readiness.HawkesFit {
			return nil
		}

		return normalization.ratio(raw, float64(outcome.EventCount))
	case types.MetricSpectralRadius:
		if unit != types.UnitDimensionless || !outcome.Readiness.HawkesFit {
			return nil
		}

		if raw < 0 || raw >= hawkesCriticalBranch {
			return nil
		}

		return normalization.dimensionless(raw)
	case types.MetricImmediateOffspring, types.MetricTotalDescendants:
		if unit != types.UnitDimensionless || !outcome.Readiness.HawkesFit || raw < 0 {
			return nil
		}

		return normalization.dimensionless(raw)
	default:
		return nil
	}
}

/*
eventCount measures total support against the fitted estimator requirement and
side counts as observed shares. A lone event is explicitly provisional and is
not enough to establish either scale.
*/
func (normalization normalizer) eventCount(
	outcome excitation.Outcome,
	side types.MeasurementSide,
	raw float64,
) *float64 {
	if !outcome.Readiness.Observation || outcome.EventCount <= 1 || raw < 0 {
		return nil
	}

	if side == types.SideNone {
		return normalization.ratio(raw, float64(outcome.MinimumFitEvents))
	}

	if side != types.SideBuy && side != types.SideSell {
		return nil
	}

	return normalization.ratio(raw, float64(outcome.EventCount))
}

/*
arrival compares each empirical marked-arrival rate to the average marked rate
before fitting and to its same-side immigrant intensity afterward.
*/
func (normalization normalizer) arrival(
	outcome excitation.Outcome,
	side types.MeasurementSide,
	raw float64,
) *float64 {
	if !outcome.Readiness.Intensity || outcome.EventCount <= 1 || raw < 0 {
		return nil
	}

	if outcome.Readiness.HawkesFit {
		return normalization.sameSideDeviation(outcome, side, raw)
	}

	total := outcome.BuyArrivalRate + outcome.SellArrivalRate

	return normalization.deviation(raw, total/hawkesMarkedSides)
}

/*
sameSideDeviation measures an observed or conditional rate against the fitted
immigrant intensity for that same mark.
*/
func (normalization normalizer) sameSideDeviation(
	outcome excitation.Outcome,
	side types.MeasurementSide,
	raw float64,
) *float64 {
	if raw < 0 {
		return nil
	}

	switch side {
	case types.SideBuy:
		return normalization.deviation(raw, outcome.Fit.MuX)
	case types.SideSell:
		return normalization.deviation(raw, outcome.Fit.MuY)
	default:
		return nil
	}
}

/*
conditionalIntensity enforces the nonnegative-kernel Hawkes contract that a
conditional rate cannot sit below its immigrant baseline.
*/
func (normalization normalizer) conditionalIntensity(
	outcome excitation.Outcome,
	side types.MeasurementSide,
	raw float64,
) *float64 {
	baseline := 0.0

	switch side {
	case types.SideBuy:
		baseline = outcome.Fit.MuX
	case types.SideSell:
		baseline = outcome.Fit.MuY
	default:
		return nil
	}

	if raw < baseline {
		return nil
	}

	return normalization.deviation(raw, baseline)
}

/*
baselineShare reports each immigrant intensity as its share of total fitted
immigrant flow. The two sides therefore remain comparable without treating a
rate as if it were already unitless.
*/
func (normalization normalizer) baselineShare(
	outcome excitation.Outcome,
	raw float64,
) *float64 {
	if raw < 0 {
		return nil
	}

	return normalization.ratio(raw, outcome.Fit.MuX+outcome.Fit.MuY)
}

func (normalization normalizer) deviation(raw, reference float64) *float64 {
	if !finite(reference) || reference <= 0 {
		return nil
	}

	value := (raw - reference) / reference

	if !finite(value) {
		return nil
	}

	return &value
}

func (normalization normalizer) ratio(raw, reference float64) *float64 {
	if !finite(reference) || reference <= 0 {
		return nil
	}

	value := raw / reference

	if !finite(value) {
		return nil
	}

	return &value
}

func (normalization normalizer) dimensionless(raw float64) *float64 {
	if !finite(raw) {
		return nil
	}

	value := raw

	return &value
}

func observationHorizon(outcome excitation.Outcome) time.Duration {
	if outcome.Horizon > 0 {
		return outcome.Horizon
	}

	if outcome.ObservedFrom.IsZero() || !outcome.At.After(outcome.ObservedFrom) {
		return 0
	}

	return outcome.At.Sub(outcome.ObservedFrom)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
