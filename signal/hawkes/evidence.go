package hawkes

import (
	"time"

	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/types"
)

/*
measurements emits the complete Hawkes metric set every observation. Arrival rates,
conditional intensities, and fit parameters publish as zero with provisional
validity until Intensity / HawkesFit readiness; static parameters stay anchored
to their fit epoch while likelihood comparisons use the evaluation interval.
*/
func (signal *Signal) measurements(
	symbol string,
	outcome excitation.Outcome,
) []*types.Measurement {
	from, through := signal.observationInterval(outcome)
	measurement := &types.Measurement{
		Source:       types.SourceHawkes,
		Symbol:       symbol,
		At:           through,
		ObservedFrom: from,
		Horizon:      through.Sub(from),
		Maturity:     outcome.Maturity,
		Metrics:      make(map[string]types.MetricSample, 16),
	}

	if outcome.Readiness.HawkesFit {
		signal.applyFitEvaluation(measurement, outcome)
	}

	signal.putObservedMetrics(measurement, outcome)
	signal.putFitMetrics(measurement, outcome)

	return []*types.Measurement{measurement}
}

/*
putObservedMetrics records the directly observed marked-arrival evidence before
adding any fitted Hawkes state.
*/
func (signal *Signal) putObservedMetrics(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
	signal.putMetric(
		measurement, outcome,
		types.MetricEventCount, types.SideNone,
		types.UnitCount, float64(outcome.EventCount),
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricEventCount, types.SideBuy,
		types.UnitCount, float64(outcome.BuyEventCount),
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricEventCount, types.SideSell,
		types.UnitCount, float64(outcome.SellEventCount),
	)

	buyRate := outcome.BuyArrivalRate
	sellRate := outcome.SellArrivalRate

	if !outcome.Readiness.Intensity {
		buyRate = 0
		sellRate = 0
	}

	signal.putMetric(
		measurement, outcome,
		types.MetricArrivalRate, types.SideBuy,
		types.UnitEventsPerSecond, buyRate,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricArrivalRate, types.SideSell,
		types.UnitEventsPerSecond, sellRate,
	)

	buyIntensity := outcome.Fit.IntensityX
	sellIntensity := outcome.Fit.IntensityY

	if !outcome.Readiness.HawkesFit {
		buyIntensity = 0
		sellIntensity = 0
	}

	signal.putMetric(
		measurement, outcome,
		types.MetricConditionalIntensity, types.SideBuy,
		types.UnitEventsPerSecond, buyIntensity,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricConditionalIntensity, types.SideSell,
		types.UnitEventsPerSecond, sellIntensity,
	)
}

/*
putFitMetrics records the identified self-excitation state once HawkesFit
readiness has been reached, while keeping the same metric keys explicit before
that epoch.
*/
func (signal *Signal) putFitMetrics(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
	muX := 0.0
	muY := 0.0
	alphaXX := 0.0
	alphaXY := 0.0
	alphaYX := 0.0
	alphaYY := 0.0
	beta := 0.0
	spectral := 0.0
	poissonDelta := 0.0
	crossSelfDelta := 0.0
	buyOffspring := 0.0
	sellOffspring := 0.0
	buyDescendants := 0.0
	sellDescendants := 0.0

	if outcome.Readiness.HawkesFit {
		muX = outcome.Fit.MuX
		muY = outcome.Fit.MuY
		alphaXX = outcome.Fit.AlphaXX
		alphaXY = outcome.Fit.AlphaXY
		alphaYX = outcome.Fit.AlphaYX
		alphaYY = outcome.Fit.AlphaYY
		beta = outcome.Fit.Beta
		spectral = outcome.Fit.SpectralRadius
		poissonDelta = outcome.HawkesPoissonLogLikelihoodDelta
		crossSelfDelta = outcome.CrossSelfLogLikelihoodDelta
		buyOffspring = outcome.ImmediateBuyOffspring
		sellOffspring = outcome.ImmediateSellOffspring
		buyDescendants = outcome.TotalBuyDescendants
		sellDescendants = outcome.TotalSellDescendants
	}

	kernelMemory := 0.0

	if beta > 0 {
		kernelMemory = 1 / beta
	}

	signal.putMetric(
		measurement, outcome,
		types.MetricBaselineIntensity, types.SideBuy,
		types.UnitEventsPerSecond, muX,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricBaselineIntensity, types.SideSell,
		types.UnitEventsPerSecond, muY,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricExcitationAmplitude, types.SideBuyToBuy,
		types.UnitEventsPerSecond, alphaXX,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricExcitationAmplitude, types.SideSellToBuy,
		types.UnitEventsPerSecond, alphaXY,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricExcitationAmplitude, types.SideBuyToSell,
		types.UnitEventsPerSecond, alphaYX,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricExcitationAmplitude, types.SideSellToSell,
		types.UnitEventsPerSecond, alphaYY,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricDecayRate, types.SideNone,
		types.UnitInverseSecond, beta,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricKernelMemory, types.SideNone,
		types.UnitSecond, kernelMemory,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricSpectralRadius, types.SideNone,
		types.UnitDimensionless, spectral,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricHawkesPoissonDelta, types.SideNone,
		types.UnitNat, poissonDelta,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricCrossSelfDelta, types.SideNone,
		types.UnitNat, crossSelfDelta,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricImmediateOffspring, types.SideBuy,
		types.UnitDimensionless, buyOffspring,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricImmediateOffspring, types.SideSell,
		types.UnitDimensionless, sellOffspring,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricTotalDescendants, types.SideBuy,
		types.UnitDimensionless, buyDescendants,
	)
	signal.putMetric(
		measurement, outcome,
		types.MetricTotalDescendants, types.SideSell,
		types.UnitDimensionless, sellDescendants,
	)
}

/*
putMetric writes one Hawkes sample with normalization owned by the signal.
*/
func (signal *Signal) putMetric(
	measurement *types.Measurement,
	outcome excitation.Outcome,
	metric types.MetricType,
	side types.MeasurementSide,
	unit types.MeasurementUnit,
	raw float64,
) {
	measurement.Metrics[types.MetricKey(metric, side)] = types.MetricSample{
		Raw:        raw,
		Normalized: signal.normalize.value(outcome, metric, side, unit, raw),
		Unit:       unit,
	}
}

/*
applyFitEvaluation anchors fitted parameters to the epoch that established
them without replacing the current empirical observation interval.
*/
func (signal *Signal) applyFitEvaluation(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
	measurement.At = outcome.FitAt
	measurement.ObservedFrom = outcome.FitObservedFrom
	measurement.Horizon = outcome.FitAt.Sub(outcome.FitObservedFrom)
}

/*
observationInterval resolves the empirical Hawkes window. A missing origin is
derived from Horizon; producers must not emit ObservedFrom after At.
*/
func (signal *Signal) observationInterval(
	outcome excitation.Outcome,
) (time.Time, time.Time) {
	from := outcome.ObservedFrom
	through := outcome.At

	if from.IsZero() && !through.IsZero() {
		from = through.Add(-outcome.Horizon)
	}

	return from, through
}
