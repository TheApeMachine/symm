package hawkes

import (
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/types"
)

/*
Evidence maps estimator output onto the shared numerical measurement contract.
It emits separate measurements because model parameters with different units,
sides, and subjects must remain independently addressable by logic.
*/
type Evidence struct{}

/*
NewEvidence returns a stateless Hawkes evidence mapper.
*/
func NewEvidence() *Evidence {
	return &Evidence{}
}

/*
Measure emits empirical evidence first, updates conditional intensities from a
retained valid fit, and publishes parameter evidence only on an actual refit
epoch. This keeps a current intensity from masquerading as a newly estimated
model.
*/
func (evidence *Evidence) Measure(
	symbol string,
	outcome excitation.Outcome,
) []*types.Measurement {
	measurements := []*types.Measurement{
		evidence.observation(
			symbol,
			outcome,
			types.MetricEventCount,
			types.SubjectTradeArrivals,
			types.SideNone,
			types.UnitCount,
			float64(outcome.EventCount),
		),
		evidence.observation(
			symbol,
			outcome,
			types.MetricEventCount,
			types.SubjectTradeArrivals,
			types.SideBuy,
			types.UnitCount,
			float64(outcome.BuyEventCount),
		),
		evidence.observation(
			symbol,
			outcome,
			types.MetricEventCount,
			types.SubjectTradeArrivals,
			types.SideSell,
			types.UnitCount,
			float64(outcome.SellEventCount),
		),
	}

	if !outcome.Readiness.Intensity {
		return measurements
	}

	measurements = append(measurements,
		evidence.intensity(
			symbol, outcome, types.SideBuy, outcome.BuyArrivalRate,
		),
		evidence.intensity(
			symbol, outcome, types.SideSell, outcome.SellArrivalRate,
		),
	)

	if !outcome.Readiness.HawkesFit {
		return measurements
	}

	measurements = append(measurements,
		evidence.conditional(
			symbol, outcome, types.SideBuy, outcome.Fit.IntensityX,
		),
		evidence.conditional(
			symbol, outcome, types.SideSell, outcome.Fit.IntensityY,
		),
	)

	if !outcome.Readiness.ModelUpdated {
		return measurements
	}

	return append(measurements, evidence.model(symbol, outcome)...)
}

func (evidence *Evidence) observation(
	symbol string,
	outcome excitation.Outcome,
	metric types.MetricType,
	subject types.SubjectType,
	side types.MeasurementSide,
	unit types.MeasurementUnit,
	raw float64,
) *types.Measurement {
	return evidence.measurement(symbol, outcome, metric, subject, side, unit, raw,
		types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		},
	)
}

func (evidence *Evidence) intensity(
	symbol string,
	outcome excitation.Outcome,
	side types.MeasurementSide,
	raw float64,
) *types.Measurement {
	reason := outcome.Readiness.Reason

	if outcome.Readiness.HawkesFit {
		reason = ""
	}

	return evidence.measurement(
		symbol,
		outcome,
		types.MetricArrivalRate,
		types.SubjectTradeArrivals,
		side,
		types.UnitEventsPerSecond,
		raw,
		types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessIntensity,
			Reason:    reason,
		},
	)
}

func (evidence *Evidence) conditional(
	symbol string,
	outcome excitation.Outcome,
	side types.MeasurementSide,
	raw float64,
) *types.Measurement {
	measurement := evidence.measurement(
		symbol,
		outcome,
		types.MetricConditionalIntensity,
		types.SubjectHawkesProcess,
		side,
		types.UnitEventsPerSecond,
		raw,
		types.MeasurementValidity{
			State:     types.ValidityProvisional,
			Readiness: types.ReadinessModel,
			Reason:    outcome.Readiness.Reason,
		},
	)
	measurement.Scale = evidence.fitScale(outcome)

	return measurement
}

func (evidence *Evidence) model(
	symbol string,
	outcome excitation.Outcome,
) []*types.Measurement {
	return []*types.Measurement{
		evidence.modelValue(symbol, outcome, types.MetricBaselineIntensity,
			types.SubjectHawkesProcess, types.SideBuy,
			types.UnitEventsPerSecond, outcome.Fit.MuX),
		evidence.modelValue(symbol, outcome, types.MetricBaselineIntensity,
			types.SubjectHawkesProcess, types.SideSell,
			types.UnitEventsPerSecond, outcome.Fit.MuY),
		evidence.modelValue(symbol, outcome, types.MetricExcitationAmplitude,
			types.SubjectHawkesKernel, types.SideBuyToBuy,
			types.UnitEventsPerSecond, outcome.Fit.AlphaXX),
		evidence.modelValue(symbol, outcome, types.MetricExcitationAmplitude,
			types.SubjectHawkesKernel, types.SideSellToBuy,
			types.UnitEventsPerSecond, outcome.Fit.AlphaXY),
		evidence.modelValue(symbol, outcome, types.MetricExcitationAmplitude,
			types.SubjectHawkesKernel, types.SideBuyToSell,
			types.UnitEventsPerSecond, outcome.Fit.AlphaYX),
		evidence.modelValue(symbol, outcome, types.MetricExcitationAmplitude,
			types.SubjectHawkesKernel, types.SideSellToSell,
			types.UnitEventsPerSecond, outcome.Fit.AlphaYY),
		evidence.modelValue(symbol, outcome, types.MetricDecayRate,
			types.SubjectHawkesKernel, types.SideNone,
			types.UnitInverseSecond, outcome.Fit.Beta),
		evidence.modelValue(symbol, outcome, types.MetricKernelMemory,
			types.SubjectHawkesKernel, types.SideNone,
			types.UnitSecond, 1/outcome.Fit.Beta),
		evidence.modelValue(symbol, outcome, types.MetricSpectralRadius,
			types.SubjectHawkesProcess, types.SideNone,
			types.UnitDimensionless, outcome.Fit.SpectralRadius),
		evidence.modelValue(symbol, outcome, types.MetricHawkesPoissonDelta,
			types.SubjectHawkesFit, types.SideNone,
			types.UnitNat, outcome.HawkesPoissonLogLikelihoodDelta),
		evidence.modelValue(symbol, outcome, types.MetricCrossSelfDelta,
			types.SubjectHawkesFit, types.SideNone,
			types.UnitNat, outcome.CrossSelfLogLikelihoodDelta),
		evidence.modelValue(symbol, outcome, types.MetricImmediateOffspring,
			types.SubjectHawkesProcess, types.SideBuy,
			types.UnitDimensionless, outcome.ImmediateBuyOffspring),
		evidence.modelValue(symbol, outcome, types.MetricImmediateOffspring,
			types.SubjectHawkesProcess, types.SideSell,
			types.UnitDimensionless, outcome.ImmediateSellOffspring),
		evidence.modelValue(symbol, outcome, types.MetricTotalDescendants,
			types.SubjectHawkesProcess, types.SideBuy,
			types.UnitDimensionless, outcome.TotalBuyDescendants),
		evidence.modelValue(symbol, outcome, types.MetricTotalDescendants,
			types.SubjectHawkesProcess, types.SideSell,
			types.UnitDimensionless, outcome.TotalSellDescendants),
	}
}

func (evidence *Evidence) modelValue(
	symbol string,
	outcome excitation.Outcome,
	metric types.MetricType,
	subject types.SubjectType,
	side types.MeasurementSide,
	unit types.MeasurementUnit,
	raw float64,
) *types.Measurement {
	measurement := evidence.measurement(symbol, outcome, metric, subject, side, unit, raw,
		types.MeasurementValidity{
			State:     types.ValidityProvisional,
			Readiness: types.ReadinessModel,
			Reason:    outcome.Readiness.Reason,
		},
	)
	measurement.Scale = evidence.fitScale(outcome)

	return measurement
}

func (evidence *Evidence) fitScale(
	outcome excitation.Outcome,
) types.ScaleReference {
	return types.ScaleReference{
		Kind:    types.ScaleObservationWindow,
		From:    outcome.FitObservedFrom,
		Through: outcome.FitAt,
	}
}

func (evidence *Evidence) measurement(
	symbol string,
	outcome excitation.Outcome,
	metric types.MetricType,
	subject types.SubjectType,
	side types.MeasurementSide,
	unit types.MeasurementUnit,
	raw float64,
	validity types.MeasurementValidity,
) *types.Measurement {
	return &types.Measurement{
		Source:       types.SourceHawkes,
		Metric:       metric,
		Subject:      subject,
		Stream:       "trades",
		Symbol:       symbol,
		Side:         side,
		At:           outcome.At,
		ObservedFrom: outcome.ObservedFrom,
		Horizon:      outcome.Horizon,
		Unit:         unit,
		Raw:          raw,
		Normalized:   0,
		Maturity:     outcome.Maturity,
		Uncertainty: types.MeasurementUncertainty{
			Available: false,
		},
		Validity: validity,
		Scale: types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    outcome.ObservedFrom,
			Through: outcome.At,
		},
	}
}
