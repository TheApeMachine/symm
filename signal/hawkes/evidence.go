package hawkes

import (
	"time"

	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/types"
)

/*
Evidence maps estimator output onto the shared numerical measurement contract.
It emits separate measurements because model parameters with different units,
sides, and subjects must remain independently addressable by logic.
*/
type Evidence struct {
	normalize normalizer
}

/*
NewEvidence returns a stateless Hawkes evidence mapper.
*/
func NewEvidence() *Evidence {
	return &Evidence{}
}

/*
Measure emits empirical evidence first, then conditional intensities and the
retained fit once HawkesFit is true. Static parameters stay anchored to their
fit epoch while likelihood comparisons use the current evaluation interval.
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

	return append(measurements, evidence.model(symbol, outcome)...)
}

/*
observation builds empirical Hawkes evidence from event counts so observations
remain distinct from fitted estimates.
*/
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

/*
intensity builds arrival-intensity evidence only when the process has
sufficient readiness.
*/
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

/*
conditional builds conditional-intensity evidence from the retained Hawkes fit
so current excitation remains explicit.
*/
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
	evidence.applyFitEvaluation(measurement, outcome)

	return measurement
}

/*
model publishes fitted Hawkes parameters and current fit-quality comparisons
without assigning the evaluation statistics to the older parameter epoch.
*/
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

/*
modelValue anchors parameters to their fit epoch and likelihood comparisons to
the current stream on which that retained fit was evaluated.
*/
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
	if subject == types.SubjectHawkesFit {
		evidence.applyFitEvaluation(measurement, outcome)

		return measurement
	}

	evidence.applyFitEpoch(measurement, outcome)

	return measurement
}

/*
applyFitEpoch anchors fitted parameters to their retained interval, deriving a
missing origin from the observation horizon without moving the fit epoch.
*/
func (evidence *Evidence) applyFitEpoch(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
	from := outcome.FitObservedFrom
	through := outcome.FitAt

	if from.IsZero() {
		from = outcome.ObservedFrom
	}

	if through.IsZero() {
		through = outcome.At
	}

	if from.IsZero() && !through.IsZero() {
		from = through.Add(-outcome.Horizon)
	}

	measurement.ObservedFrom = from
	measurement.At = through
	measurement.Horizon = through.Sub(from)
	measurement.Scale = types.ScaleReference{
		Kind:    types.ScaleObservationWindow,
		From:    from,
		Through: through,
	}
}

/*
applyFitEvaluation anchors retained-fit intensities from the fit origin through
the current evaluation horizon.
*/
func (evidence *Evidence) applyFitEvaluation(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
	from := outcome.FitObservedFrom

	if from.IsZero() {
		from = outcome.ObservedFrom
	}

	through := outcome.At

	if from.IsZero() && !through.IsZero() {
		from = through.Add(-outcome.Horizon)
	}

	measurement.ObservedFrom = from
	measurement.At = through
	measurement.Horizon = through.Sub(from)
	measurement.Scale = types.ScaleReference{
		Kind:    types.ScaleObservationWindow,
		From:    from,
		Through: through,
	}
}

/*
measurement assembles one Hawkes measurement from evidence already owned by
the signal.
*/
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
	from, through := evidence.observationInterval(outcome)

	return &types.Measurement{
		Source:       types.SourceHawkes,
		Metric:       metric,
		Subject:      subject,
		Stream:       types.Hawkes,
		Symbol:       symbol,
		Side:         side,
		At:           through,
		ObservedFrom: from,
		Horizon:      through.Sub(from),
		Unit:         unit,
		Raw:          raw,
		Normalized:   evidence.normalize.value(outcome, metric, side, unit, raw),
		Maturity:     outcome.Maturity,
		Validity:     validity,
		Scale: types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    from,
			Through: through,
		},
	}
}

/*
observationInterval resolves the empirical Hawkes window. A missing origin is
derived from Horizon; producers must not emit ObservedFrom after At.
*/
func (evidence *Evidence) observationInterval(
	outcome excitation.Outcome,
) (time.Time, time.Time) {
	from := outcome.ObservedFrom
	through := outcome.At

	if from.IsZero() && !through.IsZero() {
		from = through.Add(-outcome.Horizon)
	}

	return from, through
}
