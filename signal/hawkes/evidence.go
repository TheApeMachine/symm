package hawkes

import (
	"time"

	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/types"
)

/*
Evidence maps estimator output onto the shared numerical measurement contract.
It emits one source×symbol row whose Metrics map keeps every Hawkes quantity
independently addressable by logic and UI wire consumers.
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
Measure emits the complete Hawkes metric set every observation. Arrival rates,
conditional intensities, and fit parameters publish as zero with provisional
validity until Intensity / HawkesFit readiness; static parameters stay anchored
to their fit epoch while likelihood comparisons use the evaluation interval.
*/
func (evidence *Evidence) Measure(
	symbol string,
	outcome excitation.Outcome,
) []*types.Measurement {
	from, through := evidence.observationInterval(outcome)
	measurement := &types.Measurement{
		Source:       types.SourceHawkes,
		Symbol:       symbol,
		At:           through,
		ObservedFrom: from,
		Horizon:      through.Sub(from),
		Maturity:     outcome.Maturity,
		Validity:     evidence.validity(outcome),
		Scale: types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    from,
			Through: through,
		},
		Metrics: make(map[string]types.MetricSample, 16),
	}

	if outcome.Readiness.HawkesFit {
		evidence.applyFitEvaluation(measurement, outcome)
	}

	evidence.putObservations(measurement, outcome)
	evidence.putIntensities(measurement, outcome)
	evidence.putConditionals(measurement, outcome)
	evidence.putModel(measurement, outcome)

	return []*types.Measurement{measurement}
}

/*
validity summarizes estimator readiness for the merged Hawkes row.
*/
func (evidence *Evidence) validity(
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
func (evidence *Evidence) putObservations(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
	evidence.putMetric(
		measurement, outcome,
		types.MetricEventCount, types.SideNone,
		types.UnitCount, float64(outcome.EventCount),
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricEventCount, types.SideBuy,
		types.UnitCount, float64(outcome.BuyEventCount),
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricEventCount, types.SideSell,
		types.UnitCount, float64(outcome.SellEventCount),
	)
}

/*
putIntensities records arrival-intensity evidence, zeroing raw values before
Intensity readiness while preserving metric identity.
*/
func (evidence *Evidence) putIntensities(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
	buyRate := outcome.BuyArrivalRate
	sellRate := outcome.SellArrivalRate

	if !outcome.Readiness.Intensity {
		buyRate = 0
		sellRate = 0
	}

	evidence.putMetric(
		measurement, outcome,
		types.MetricArrivalRate, types.SideBuy,
		types.UnitEventsPerSecond, buyRate,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricArrivalRate, types.SideSell,
		types.UnitEventsPerSecond, sellRate,
	)
}

/*
putConditionals records conditional-intensity evidence from the retained fit.
*/
func (evidence *Evidence) putConditionals(
	measurement *types.Measurement,
	outcome excitation.Outcome,
) {
	buyIntensity := outcome.Fit.IntensityX
	sellIntensity := outcome.Fit.IntensityY

	if !outcome.Readiness.HawkesFit {
		buyIntensity = 0
		sellIntensity = 0
	}

	evidence.putMetric(
		measurement, outcome,
		types.MetricConditionalIntensity, types.SideBuy,
		types.UnitEventsPerSecond, buyIntensity,
	)
	evidence.putMetric(
		measurement, outcome,
		types.MetricConditionalIntensity, types.SideSell,
		types.UnitEventsPerSecond, sellIntensity,
	)
}

/*
putMetric writes one Hawkes sample with normalization owned by the mapper.
*/
func (evidence *Evidence) putMetric(
	measurement *types.Measurement,
	outcome excitation.Outcome,
	metric types.MetricType,
	side types.MeasurementSide,
	unit types.MeasurementUnit,
	raw float64,
) {
	measurement.Metrics[types.MetricKey(metric, side)] = types.MetricSample{
		Raw:        raw,
		Normalized: evidence.normalize.value(outcome, metric, side, unit, raw),
		Unit:       unit,
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
