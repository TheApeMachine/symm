package derivatives

import (
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
BuildMeasurement derives the dynamic multi-dimensional derivatives metrics
and regime hypotheses from the current symbol state.
*/
func BuildMeasurement(
	sourceName string,
	symbolName string,
	state *SymbolState,
	at time.Time,
) *nmtypes.Measurement {
	state.mu.Lock()
	defer state.mu.Unlock()

	totalVolume := state.FuturesBuyVolume + state.FuturesSellVolume
	aggressorImbalance := 0.0

	if totalVolume > 0 {
		aggressorImbalance = (state.FuturesBuyVolume - state.FuturesSellVolume) / totalVolume
	}

	liqTotal := state.LiqBuyVolume + state.LiqSellVolume
	liqIntensity := 0.0

	if totalVolume > 0 {
		liqIntensity = liqTotal / totalVolume
	}

	priceVel := 0.0

	if state.LastSpotPrice > 0 && len(state.PriceHistory) >= 2 {
		prevSpot := state.PriceHistory[0].spot

		if prevSpot > 0 {
			priceVel = (state.LastSpotPrice - prevSpot) / prevSpot
		}
	}

	oiVel := state.OIVelocity
	cvd := state.FuturesCVD

	scoreIgnition := math.Max(0, math.Min(1, 0.5+0.5*math.Tanh(priceVel*50.0+oiVel*10.0+cvd*0.01)))
	scoreSqueeze := math.Max(0, math.Min(1, 0.5+0.5*math.Tanh(priceVel*50.0-oiVel*10.0+state.LiqBuyVolume*0.1)))
	scoreBuildup := math.Max(0, math.Min(1, 0.5+0.5*math.Tanh(-priceVel*50.0+oiVel*10.0-cvd*0.01)))
	scoreDeleveraging := math.Max(0, math.Min(1, 0.5+0.5*math.Tanh(-priceVel*50.0-oiVel*10.0+state.LiqSellVolume*0.1)))
	scoreDecoupling := math.Max(0, math.Min(1, 0.5+0.5*math.Tanh(state.TripartiteDiv*100.0+math.Abs(state.Basis)*50.0)))

	scores := []float64{scoreIgnition, scoreSqueeze, scoreBuildup, scoreDeleveraging, scoreDecoupling}
	separation := computeSeparation(scores)

	eventNano := at.UnixNano()

	if eventNano == 0 {
		eventNano = time.Now().UTC().UnixNano()
	}

	measurement := nmtypes.NewMeasurement(
		uuid.NewString(),
		sourceName,
		eventNano,
		eventNano,
	).AddMetrics(
		nmtypes.NewMetric(string(types.MetricFuturesOI), state.LastOpenInterest, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesOIVelocity), state.OIVelocity, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescalePerSecond,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesOIAcceleration), state.OIAcceleration, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescalePerSecond,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesBasis), state.Basis, nmtypes.Descriptor{
			Unit:      nmtypes.UnitPercent,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesBasisVelocity), state.BasisVelocity, nmtypes.Descriptor{
			Unit:      nmtypes.UnitPercent,
			Timescale: nmtypes.TimescalePerSecond,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesIndexBasis), state.IndexBasis, nmtypes.Descriptor{
			Unit:      nmtypes.UnitPercent,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesTripartiteDivergence), state.TripartiteDiv, nmtypes.Descriptor{
			Unit:      nmtypes.UnitPercent,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesCVD), state.FuturesCVD, nmtypes.Descriptor{
			Unit:      nmtypes.UnitQuoteCurrency,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricFuturesAggressorImbalance), aggressorImbalance, aggressorImbalance, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesLiquidationBuy), state.LiqBuyVolume, nmtypes.Descriptor{
			Unit:      nmtypes.UnitQuoteCurrency,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesLiquidationSell), state.LiqSellVolume, nmtypes.Descriptor{
			Unit:      nmtypes.UnitQuoteCurrency,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricFuturesLiquidationIntensity), liqIntensity, liqIntensity, nmtypes.Descriptor{
			Unit:      nmtypes.UnitPercent,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric(string(types.MetricFuturesLeadLagTau), state.LeadLagTau, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricFuturesLeadCorrelation), state.LeadLagCorr, state.LeadLagCorr, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricLeveragedIgnition), scoreIgnition, scoreIgnition, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricShortSqueeze), scoreSqueeze, scoreSqueeze, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricAdverseLeverageBuildup), scoreBuildup, scoreBuildup, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricLongDeleveraging), scoreDeleveraging, scoreDeleveraging, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricDerivativesDecoupling), scoreDecoupling, scoreDecoupling, nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
	)

	measurement.StampQuality(separation, float64(len(state.PriceHistory)+1))
	return measurement
}

func computeSeparation(scores []float64) float64 {
	if len(scores) < 2 {
		return 0
	}

	max1 := 0.0
	max2 := 0.0

	for _, s := range scores {
		if s > max1 {
			max2 = max1
			max1 = s
			continue
		}

		if s > max2 {
			max2 = s
		}
	}

	return statistic.StandardSeparation(max1 - max2)
}
