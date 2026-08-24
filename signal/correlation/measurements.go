package correlation

import (
	"time"

	"github.com/google/uuid"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal) measurement(
	symbol string,
	at time.Time,
	output nmtypes.Frame,
	measured bool,
	separation float64,
	support float64,
) *nmtypes.Measurement {
	descriptor := nmtypes.Descriptor{
		Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous,
	}
	measurement := nmtypes.NewMeasurement(uuid.NewString(), signal.Name(), at.UnixNano(), at.UnixNano())
	path, found := signal.number.Project(symbol)

	if found {
		observedFrom, _, hasObservation := temporal.PathSample(&path, 0)

		if hasObservation {
			measurement.ObservedFrom = time.Unix(0, observedFrom)
		}
	}

	/*
		Before the cross-section has any peer evidence the correlation scores do
		not exist yet. Emit honest neutral readings — zero correlation, zero
		energy, zero scores — and let hypothesis_separation (zero) and maturity
		(low) mark how much those readings are worth. A silent skip would hide
		the observation and stall downstream consumers waiting for first data.
	*/
	measurement.AddMetrics(
		nmtypes.NewMetric(string(types.MetricCorrelation), metricValue(output, nmcorrelation.SymbolCohortCorrelation, measured), descriptor),
		nmtypes.NewMetric(string(types.MetricSigned), metricValue(output, nmcorrelation.SymbolSignedCorrelation, measured), descriptor),
		nmtypes.NewMetric(string(types.MetricRelativeEnergy), metricValue(output, nmcorrelation.SymbolRelativeEnergy, measured), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricHerdScore), metricValue(output, nmcorrelation.SymbolHerd, measured), metricValue(output, nmcorrelation.SymbolHerd, measured), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricAlphaScore), metricValue(output, nmcorrelation.SymbolAlpha, measured), metricValue(output, nmcorrelation.SymbolAlpha, measured), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricNoiseScore), metricValue(output, nmcorrelation.SymbolNoise, measured), metricValue(output, nmcorrelation.SymbolNoise, measured), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricStressScore), metricValue(output, nmcorrelation.SymbolStress, measured), metricValue(output, nmcorrelation.SymbolStress, measured), descriptor),
	)
	measurement.StampQuality(separation, support)

	return measurement
}

func (signal *Signal) support(symbol string) float64 {
	path, found := signal.number.Project(symbol)

	if !found {
		return 0
	}

	value, _ := path.Get(nmtypes.SampleCount)

	return value
}

func metricValue(
	frame nmtypes.Frame,
	symbol nmtypes.Symbol,
	measured bool,
) float64 {
	if !measured {
		return 0
	}

	value, found := frame.Get(symbol)

	if found {
		return value
	}

	return 0
}
