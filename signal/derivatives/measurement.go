package derivatives

import (
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
DerivativesData carries the evaluated multi-dimensional state of the derivatives pipeline.
*/
type DerivativesData struct {
	OI                     float64
	OIVelocity             float64
	OIAcceleration         float64
	Basis                  float64
	BasisVelocity          float64
	IndexBasis             float64
	TripartiteDivergence   float64
	CVD                    float64
	AggressorImbalance     float64
	LiquidationBuy         float64
	LiquidationSell        float64
	LiquidationIntensity   float64
	LeveragedIgnition      float64
	ShortSqueeze           float64
	AdverseLeverageBuildup float64
	LongDeleveraging       float64
	DerivativesDecoupling  float64
	SampleCount            float64
}

/*
BuildMeasurement projects the evaluated derivatives metrics into a typed
multi-dimensional measurement artifact.
*/
func BuildMeasurement(
	sourceName string,
	symbolName string,
	at time.Time,
	data DerivativesData,
) (*nmtypes.Measurement, error) {
	if at.IsZero() {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"derivatives: timestamp is zero for symbol "+symbolName,
			nil,
		))
	}

	eventNano := at.UnixNano()

	measurement := nmtypes.NewMeasurement(
		uuid.NewString(),
		sourceName,
		eventNano,
		eventNano,
	).AddMetrics(
		nmtypes.NewMetric(string(types.MetricFuturesOI), data.OI, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesOIVelocity), data.OIVelocity, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescalePerSecond,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesOIAcceleration), data.OIAcceleration, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescalePerSecond,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesBasis), data.Basis, nmtypes.Descriptor{
			Unit:      nmtypes.UnitPercent,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesBasisVelocity), data.BasisVelocity, nmtypes.Descriptor{
			Unit:      nmtypes.UnitPercent,
			Timescale: nmtypes.TimescalePerSecond,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesIndexBasis), data.IndexBasis, nmtypes.Descriptor{
			Unit:      nmtypes.UnitPercent,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesTripartiteDivergence), data.TripartiteDivergence, nmtypes.Descriptor{
			Unit:      nmtypes.UnitPercent,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesCVD), data.CVD, nmtypes.Descriptor{
			Unit:      nmtypes.UnitQuoteCurrency,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesAggressorImbalance), data.AggressorImbalance, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesLiquidationBuy), data.LiquidationBuy, nmtypes.Descriptor{
			Unit:      nmtypes.UnitQuoteCurrency,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesLiquidationSell), data.LiquidationSell, nmtypes.Descriptor{
			Unit:      nmtypes.UnitQuoteCurrency,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesLiquidationIntensity), data.LiquidationIntensity, nmtypes.Descriptor{
			Unit:      nmtypes.UnitPercent,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricLeveragedIgnition), data.LeveragedIgnition, data.LeveragedIgnition, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricShortSqueeze), data.ShortSqueeze, data.ShortSqueeze, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricAdverseLeverageBuildup), data.AdverseLeverageBuildup, data.AdverseLeverageBuildup, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricLongDeleveraging), data.LongDeleveraging, data.LongDeleveraging, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricDerivativesDecoupling), data.DerivativesDecoupling, data.DerivativesDecoupling, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
	)

	sampleCount := data.SampleCount

	if sampleCount == 0 {
		sampleCount = 1
	}

	measurement.StampQuality(
		statistic.StandardSeparation(data.OIVelocity*50),
		sampleCount,
	)

	return measurement, nil
}
