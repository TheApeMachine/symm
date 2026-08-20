package correlation

import (
	"time"

	"github.com/google/uuid"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal) measurement(symbol string, at time.Time) (*nmtypes.Measurement, bool) {
	scores, ready := signal.section.Scores(symbol)

	if !ready {
		return nil, false
	}

	descriptor := nmtypes.Descriptor{
		Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous,
	}
	measurement := nmtypes.NewMeasurement(uuid.NewString(), signal.Name(), at.UnixNano(), at.UnixNano())
	measurement.ObservedFrom = scores.ObservedFrom
	measurement.AddMetrics(
		nmtypes.NewMetric(string(types.MetricCorrelation), scores.Correlation, descriptor),
		nmtypes.NewMetric(string(types.MetricSigned), scores.Signed, descriptor),
		nmtypes.NewMetric(string(types.MetricRelativeEnergy), scores.RelativeEnergy, descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricHerdScore), scores.Herd, scores.Herd, descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricAlphaScore), scores.Alpha, scores.Alpha, descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricNoiseScore), scores.Noise, scores.Noise, descriptor),
		nmtypes.NewNormalizedMetric(string(types.MetricStressScore), scores.Stress, scores.Stress, descriptor),
		nmtypes.NewMetric(string(types.MetricHypothesisSeparation), scores.SNR, descriptor),
	)

	return measurement, true
}
