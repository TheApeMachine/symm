package correlation

import (
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal) measurement(
	symbol string,
	at time.Time,
	output types.Frame,
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
		nmtypes.NewMetric(string(types.MetricCorrelation), metricValue(output, algo.SymbolCohortCorrelation, measured), descriptor),
		nmtypes.NewMetric(string(types.MetricSigned), metricValue(output, algo.SymbolSignedCorrelation, measured), descriptor),
		nmtypes.NewMetric(string(types.MetricRelativeEnergy), metricValue(output, algo.SymbolRelativeEnergy, measured), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricHerdScore), metricValue(output, algo.SymbolHerd, measured), metricValue(output, algo.SymbolHerd, measured), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricAlphaScore), metricValue(output, algo.SymbolAlpha, measured), metricValue(output, algo.SymbolAlpha, measured), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricNoiseScore), metricValue(output, algo.SymbolNoise, measured), metricValue(output, algo.SymbolNoise, measured), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricStressScore), metricValue(output, algo.SymbolStress, measured), metricValue(output, algo.SymbolStress, measured), descriptor),
	)
	measurement.StampQuality(separation, support)

	return measurement
}

func (signal *Signal) support(symbol string) float64 {
	path, found := signal.number.Project(symbol)

	if !found {
		return 0
	}

	value, _ := path.Get(nomagique.SampleCount)

	return value
}

func metricValue(
	frame types.Frame,
	symbol nomagique.Symbol,
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
