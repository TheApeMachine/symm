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
	output nomagique.Frame,
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

	measurement.AddMetrics(
		nmtypes.NewMetric(string(types.MetricCorrelation), output.MustGet(algo.SymbolCohortCorrelation), descriptor),
		nmtypes.NewMetric(string(types.MetricSigned), output.MustGet(algo.SymbolSignedCorrelation), descriptor),
		nmtypes.NewMetric(string(types.MetricRelativeEnergy), output.MustGet(algo.SymbolRelativeEnergy), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricHerdScore), output.MustGet(algo.SymbolHerd), output.MustGet(algo.SymbolHerd), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricAlphaScore), output.MustGet(algo.SymbolAlpha), output.MustGet(algo.SymbolAlpha), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricNoiseScore), output.MustGet(algo.SymbolNoise), output.MustGet(algo.SymbolNoise), descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricStressScore), output.MustGet(algo.SymbolStress), output.MustGet(algo.SymbolStress), descriptor),
		nmtypes.NewMetric(string(types.MetricHypothesisSeparation), output.MustGet(algo.SymbolHypothesisSeparation), descriptor),
	)

	return measurement
}
