package replay

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
entryDeployFraction scales the global position_fraction by the node's multiplier
and the structural regime. Choppy and bearish regimes deploy less capital so the
search co-evolves sizing with liquidity conditions.
*/
func entryDeployFraction(
	costs ReplayCosts,
	act reasoning.Act,
	snapshots []types.Measurement,
) float64 {
	multiplier := 1.0

	if act.Fraction > 0 {
		multiplier = act.Fraction
	}

	fraction := effectiveFraction(costs) * multiplier

	if fraction <= 0 {
		return 0
	}

	regime := perspectives.ClassifyRegime(snapshots).Regime
	scale := regimeSizeScale(regime)

	if scale > 0 && scale != 1 {
		fraction *= scale
	}

	if fraction < 0 {
		return 0
	}

	return fraction
}

func regimeSizeScale(regime types.Regime) float64 {
	config := viper.GetViper()

	switch regime {
	case types.RegimeChoppy:
		key := "trading.replay.choppy_size_scale"

		if !config.IsSet(key) {
			return 0
		}

		value := config.GetFloat64(key)

		if value <= 0 {
			return 0
		}

		return value
	case types.RegimeBearish:
		key := "trading.replay.bearish_size_scale"

		if !config.IsSet(key) {
			return 0
		}

		value := config.GetFloat64(key)

		if value <= 0 {
			return 0
		}

		return value
	default:
		return 1
	}
}
