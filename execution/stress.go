package execution

import (
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
StressMultiplier scales slippage when measurements show hostile flow or the structural
regime is adverse. Replay passes the tick snapshot; live paths combine symbol stress
readings with the published market regime.
*/
func StressMultiplier(snapshots []types.Measurement) float64 {
	if len(snapshots) == 0 {
		return 1
	}

	stressSNR := 0.0
	stressReadings := 0

	for _, measurement := range snapshots {
		if !stressCategory(measurement.Category) {
			continue
		}

		stressReadings++

		if measurement.SNR > stressSNR {
			stressSNR = measurement.SNR
		}
	}

	categoryFactor := 1.0

	if stressReadings > 0 && stressSNR > 0 {
		coverage := float64(stressReadings) / float64(len(snapshots))
		strength := stressSNR / (stressSNR + 1)
		categoryFactor = 1 + coverage*strength
	}

	return categoryFactor * RegimeHostility(perspectives.ClassifyRegime(snapshots).Regime)
}

/*
StressFromHostileSNR approximates category stress from a desk symbol-stress reading.
*/
func StressFromHostileSNR(hostileSNR float64, regime types.Regime) float64 {
	categoryFactor := 1.0

	if hostileSNR > 0 {
		categoryFactor = 1 + hostileSNR/(hostileSNR+1)
	}

	return categoryFactor * RegimeHostility(regime)
}

/*
RegimeHostility is the structural-regime adverse-selection multiplier.
*/
func RegimeHostility(regime types.Regime) float64 {
	switch regime {
	case types.RegimeBearish:
		return 1.5
	case types.RegimeChoppy:
		return 1.25
	default:
		return 1
	}
}

func stressCategory(category types.CategoryType) bool {
	switch category {
	case types.CategoryTurbulent,
		types.CategoryFrenzy,
		types.CategoryLiquidityVacuum,
		types.CategoryLiquidityShock,
		types.CategoryToxicBluff,
		types.CategorySpoofTrap,
		types.CategoryBookThinning,
		types.CategorySystemicHerd,
		types.CategoryMechanicalCollapse,
		types.CategoryDivergentStress,
		types.CategorySystemicSlump:
		return true
	default:
		return false
	}
}

/*
StressedFillQuote worsens an achieved book-walk fill for hostile flow, regime, and
depth shortfall. anchor is the book-walk price (or the side touch when no walk was
possible) — the stress component applies ON TOP of the walk, so a calm, fully
covered fill returns the walk price unchanged. Stress can only move a fill against
the taker; it must never produce a price better than the anchor. Anchoring at the
reference last/mid (the previous behavior) silently erased the bid/ask spread from
every paper and replay fill. slippageBps reports total modeled slippage (walk +
stress) for gates and telemetry.
*/
func StressedFillQuote(
	side trading.Side,
	anchor float64,
	baseSlippageBps float64,
	depthCoverage float64,
	multiplier float64,
	defaultSlippagePct float64,
) (price float64, slippageBps float64, err error) {
	basePct := baseSlippageBps / 10_000

	if basePct < 0 {
		basePct = 0
	}

	if multiplier < 1 {
		multiplier = 1
	}

	extraPct := basePct * (multiplier - 1)

	if depthCoverage > 0 && depthCoverage < 1 {
		shortfall := 1 - depthCoverage
		base := basePct

		if base <= 0 {
			base = defaultSlippagePct
		}

		shortfallMultiplier, multiplierErr := requiredDepthShortfallStressMultiplier()

		if multiplierErr != nil {
			return 0, 0, multiplierErr
		}

		extraPct += shortfall * base * multiplier * shortfallMultiplier
	}

	slippageBps = (basePct + extraPct) * 10_000

	if anchor <= 0 {
		return 0, slippageBps, nil
	}

	if side == trading.Sell {
		return anchor * (1 - extraPct), slippageBps, nil
	}

	return anchor * (1 + extraPct), slippageBps, nil
}

func requiredDepthShortfallStressMultiplier() (float64, error) {
	multiplier := viper.GetFloat64("trading.replay.depth_shortfall_stress")

	if multiplier <= 0 {
		return 0, fmt.Errorf(
			"execution: trading.replay.depth_shortfall_stress must be > 0",
		)
	}

	return multiplier, nil
}
