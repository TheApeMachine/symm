package hawkes

import (
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/types"
)

/*
putModel publishes fitted Hawkes parameters and current fit-quality comparisons.
*/
func (evidence *Evidence) putModel(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
	values := evidence.prepareModelValues(outcome)

	evidence.putBaselineIntensities(measurement, outcome, values)
	evidence.putExcitationAmplitudes(measurement, outcome, values)
	evidence.putKernelMetrics(measurement, outcome, values)
	evidence.putLikelihoodBranching(measurement, outcome, values)
}

/*
modelValues holds readiness-gated Hawkes fit parameters and comparisons.
*/
type modelValues struct {
	muX, muY                           float64
	alphaXX, alphaXY, alphaYX, alphaYY float64
	beta, kernelMemory, spectral       float64
	poissonDelta, crossSelfDelta         float64
	buyOffspring, sellOffspring          float64
	buyDescendants, sellDescendants      float64
}

/*
prepareModelValues zeroes model evidence until HawkesFit readiness is satisfied.
*/
func (evidence *Evidence) prepareModelValues(
	outcome excitation.Outcome,
) modelValues {
	values := modelValues{
		muX:             outcome.Fit.MuX,
		muY:             outcome.Fit.MuY,
		alphaXX:         outcome.Fit.AlphaXX,
		alphaXY:         outcome.Fit.AlphaXY,
		alphaYX:         outcome.Fit.AlphaYX,
		alphaYY:         outcome.Fit.AlphaYY,
		beta:            outcome.Fit.Beta,
		spectral:        outcome.Fit.SpectralRadius,
		poissonDelta:    outcome.HawkesPoissonLogLikelihoodDelta,
		crossSelfDelta:  outcome.CrossSelfLogLikelihoodDelta,
		buyOffspring:    outcome.ImmediateBuyOffspring,
		sellOffspring:   outcome.ImmediateSellOffspring,
		buyDescendants:  outcome.TotalBuyDescendants,
		sellDescendants: outcome.TotalSellDescendants,
	}

	if outcome.Readiness.HawkesFit && values.beta > 0 {
		values.kernelMemory = 1 / values.beta
	}

	if !outcome.Readiness.HawkesFit {
		values.muX, values.muY = 0, 0
		values.alphaXX, values.alphaXY, values.alphaYX, values.alphaYY = 0, 0, 0, 0
		values.beta, values.kernelMemory, values.spectral = 0, 0, 0
		values.poissonDelta, values.crossSelfDelta = 0, 0
		values.buyOffspring, values.sellOffspring = 0, 0
		values.buyDescendants, values.sellDescendants = 0, 0
	}

	return values
}

/*
putBaselineIntensities records fitted baseline arrival rates for both sides.
*/
func (evidence *Evidence) putBaselineIntensities(
	measurement *types.Measurement,
	outcome excitation.Outcome,
	values modelValues,
) {
	evidence.putMetric(
		measurement, outcome,
		types.MetricBaselineIntensity, types.SideBuy,
		types.UnitEventsPerSecond, values.muX,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricBaselineIntensity, types.SideSell,
		types.UnitEventsPerSecond, values.muY,
	)
}

/*
putExcitationAmplitudes records fitted cross- and self-excitation amplitudes.
*/
func (evidence *Evidence) putExcitationAmplitudes(
	measurement *types.Measurement,
	outcome excitation.Outcome,
	values modelValues,
) {
	evidence.putMetric(
		measurement, outcome,
		types.MetricExcitationAmplitude, types.SideBuyToBuy,
		types.UnitEventsPerSecond, values.alphaXX,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricExcitationAmplitude, types.SideSellToBuy,
		types.UnitEventsPerSecond, values.alphaXY,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricExcitationAmplitude, types.SideBuyToSell,
		types.UnitEventsPerSecond, values.alphaYX,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricExcitationAmplitude, types.SideSellToSell,
		types.UnitEventsPerSecond, values.alphaYY,
	)
}

/*
putKernelMetrics records decay rate, kernel memory, and spectral radius.
*/
func (evidence *Evidence) putKernelMetrics(
	measurement *types.Measurement,
	outcome excitation.Outcome,
	values modelValues,
) {
	evidence.putMetric(
		measurement, outcome,
		types.MetricDecayRate, types.SideNone,
		types.UnitInverseSecond, values.beta,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricKernelMemory, types.SideNone,
		types.UnitSecond, values.kernelMemory,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricSpectralRadius, types.SideNone,
		types.UnitDimensionless, values.spectral,
	)
}

/*
putLikelihoodBranching records fit comparisons and branching summaries.
*/
func (evidence *Evidence) putLikelihoodBranching(
	measurement *types.Measurement,
	outcome excitation.Outcome,
	values modelValues,
) {
	evidence.putMetric(
		measurement, outcome,
		types.MetricHawkesPoissonDelta, types.SideNone,
		types.UnitNat, values.poissonDelta,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricCrossSelfDelta, types.SideNone,
		types.UnitNat, values.crossSelfDelta,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricImmediateOffspring, types.SideBuy,
		types.UnitDimensionless, values.buyOffspring,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricImmediateOffspring, types.SideSell,
		types.UnitDimensionless, values.sellOffspring,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricTotalDescendants, types.SideBuy,
		types.UnitDimensionless, values.buyDescendants,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricTotalDescendants, types.SideSell,
		types.UnitDimensionless, values.sellDescendants,
	)
}
