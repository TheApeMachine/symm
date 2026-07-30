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
		Maturity:     signal.thesis.Tick,
		Validity:     signal.validity(outcome),
		Scale: types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    from,
			Through: through,
		},
		Metrics: make(map[string]types.MetricSample, 16),
	}

	if outcome.Readiness.HawkesFit {
		signal.applyFitEvaluation(measurement, outcome)
	}

	signal.putObservations(measurement, outcome)
	signal.putIntensities(measurement, outcome)
	signal.putConditionals(measurement, outcome)
	signal.putModel(measurement, outcome)

	return []*types.Measurement{measurement}
}

/*
validity summarizes estimator readiness for the merged Hawkes row.
*/
func (signal *Signal) validity(
	outcome excitation.Outcome,
) types.MeasurementValidity {
	if outcome.Readiness.HawkesFit {
		return types.MeasurementValidity{
			State:     types.ValidityProvisional,
			Readiness: types.ReadinessModel,
			Reason:    outcome.Readiness.Reason,
		}
	}

	validity := types.MeasurementValidity{
		State:     types.ValidityProvisional,
		Readiness: types.ReadinessIntensity,
		Reason:    outcome.Readiness.Reason,
	}

	if outcome.Readiness.Intensity {
		validity.State = types.ValidityValid
	}

	return validity
}

/*
putObservations records empirical event counts separately from fitted estimates.
*/
func (signal *Signal) putObservations(
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
}

/*
putIntensities records arrival-intensity evidence, zeroing raw values before
Intensity readiness while preserving metric identity.
*/
func (signal *Signal) putIntensities(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
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
}

/*
putConditionals records conditional-intensity evidence from the retained fit.
*/
func (signal *Signal) putConditionals(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
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
putMetric writes one Hawkes sample with normalization owned by the mapper.
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
applyFitEvaluation anchors retained-fit intensities from the fit origin through
the current evaluation horizon.
*/
func (signal *Signal) applyFitEvaluation(
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
