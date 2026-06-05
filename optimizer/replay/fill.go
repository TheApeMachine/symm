package replay

import (
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
executionFill is the replay ledger's taker-style fill quote for one quantity.
*/
type executionFill struct {
	price         float64
	slippagePct   float64
	depthCoverage float64
}

/*
takerFill walks historical L2 depth when present, otherwise crosses half the spread.
Stress multipliers and depth shortfalls inflate slippage so the optimizer cannot
assume unlimited top-of-book liquidity.
*/
func takerFill(
	costs ReplayCosts,
	measurement perspectives.Measurement,
	side trading.Side,
	quantity float64,
	snapshots []perspectives.Measurement,
) (executionFill, error) {
	if quantity <= 0 {
		return executionFill{}, fmt.Errorf("replay fill: quantity must be positive")
	}

	if measurement.Last <= 0 {
		return executionFill{}, fmt.Errorf("replay fill: missing reference price for %s", measurement.Symbol)
	}

	fill, err := walkBookFill(measurement, side, quantity)

	if err != nil {
		return executionFill{}, err
	}

	slippagePct := fill.slippageBps / 10_000
	slippagePct *= executionStressMultiplier(snapshots)

	if measurement.HasBookDepth() && fill.depthCoverage < 1 {
		shortfall := 1 - fill.depthCoverage
		slippagePct += shortfall * depthShortfallSlippagePct(costs, measurement, snapshots)
	}

	return executionFill{
		price:         fill.price,
		slippagePct:   slippagePct,
		depthCoverage: fill.depthCoverage,
	}, nil
}

/*
flatSlippagePct is the legacy half-spread model used when no book depth is present.
*/
func flatSlippagePct(
	costs ReplayCosts,
	spreadBPS float64,
	snapshots []perspectives.Measurement,
) float64 {
	return executionSlippagePct(costs, spreadBPS, snapshots)
}

func depthShortfallSlippagePct(
	costs ReplayCosts,
	measurement perspectives.Measurement,
	snapshots []perspectives.Measurement,
) float64 {
	base := flatSlippagePct(costs, measurement.SpreadBPS, snapshots)

	if base <= 0 {
		base = costs.SlippagePct
	}

	stress := executionStressMultiplier(snapshots)

	return base * stress * depthShortfallStressMultiplier()
}

func depthShortfallStressMultiplier() float64 {
	multiplier := viper.GetViper().GetFloat64("trading.replay.depth_shortfall_stress")

	if multiplier > 0 {
		return multiplier
	}

	return 8
}

func minDepthCoverage() float64 {
	coverage := viper.GetViper().GetFloat64("trading.replay.min_depth_coverage")

	if coverage > 0 {
		return coverage
	}

	return 1
}

func depthShortfallPenalty(
	slot float64,
	coverage float64,
	costs ReplayCosts,
	measurement perspectives.Measurement,
	snapshots []perspectives.Measurement,
) float64 {
	if coverage >= 1 || slot <= 0 {
		return 0
	}

	shortfall := 1 - coverage
	penalty := slot * shortfall * depthShortfallSlippagePct(costs, measurement, snapshots)

	if penalty <= 0 {
		return slot * shortfall
	}

	return penalty
}

func fillPriceFromPct(side trading.Side, reference, slippagePct float64) float64 {
	if side == trading.Sell {
		return reference * (1 - slippagePct)
	}

	return reference * (1 + slippagePct)
}
