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
model publishes fitted Hawkes parameters so downstream logic can inspect the
model producing intensities.
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
modelValue builds one fitted parameter measurement with its fit epoch and
readiness attached.
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
	evidence.applyFitEpoch(measurement, outcome)

	return measurement
}

/*
applyFitEpoch anchors fitted-parameter evidence to the retained fit interval so
graph validation does not mix a newer observation window with an older fit end.
*/
func (evidence *Evidence) applyFitEpoch(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
	from, through := evidence.fitEpochBounds(outcome)
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

	if through.Before(from) {
		from = through
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
fitEpochBounds resolves the retained Hawkes fit interval, falling back to the
current observation window when fit provenance has not been stamped yet.
*/
func (evidence *Evidence) fitEpochBounds(
	outcome excitation.Outcome,
) (time.Time, time.Time) {
	from := outcome.FitObservedFrom
	through := outcome.FitAt

	if from.IsZero() {
		from = outcome.ObservedFrom
	}

	if through.IsZero() {
		through = outcome.At
	}

	if through.Before(from) {
		from = through
	}

	return from, through
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
		Normalized:   evidence.normalized(outcome, metric, side, unit, raw),
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
observationInterval resolves the empirical Hawkes window and collapses any
backward edge to a point interval instead of publishing inverted provenance.
*/
func (evidence *Evidence) observationInterval(
	outcome excitation.Outcome,
) (time.Time, time.Time) {
	from := outcome.ObservedFrom
	through := outcome.At

	if through.Before(from) {
		from = through
	}

	return from, through
}

/*
normalized normalizes a Hawkes value against a compatible empirical reference
when that reference exists.
*/
func (evidence *Evidence) normalized(
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
		return evidence.arrivalNormalized(side, outcome, raw)
	case types.MetricConditionalIntensity:
		return evidence.intensityNormalized(side, outcome, raw)
	case types.MetricBaselineIntensity:
		return types.NormalizeFinite(raw)
	default:
		return nil
	}
}

/*
arrivalNormalized normalizes arrival rate against process baseline so excess
activity is comparable within a fit epoch.
*/
func (evidence *Evidence) arrivalNormalized(
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

	return evidence.intensityNormalized(side, outcome, raw)
}

/*
intensityNormalized normalizes conditional intensity against its arrival
baseline so excitation remains directional.
*/
func (evidence *Evidence) intensityNormalized(
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
