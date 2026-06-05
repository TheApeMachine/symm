package replay

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
entryDeployFraction scales the global position_fraction by the node's multiplier
and the structural regime. Choppy and bearish regimes deploy less capital so the
search co-evolves sizing with liquidity conditions.
*/
func entryDeployFraction(
	costs ReplayCosts,
	act perspectives.Act,
	snapshots []perspectives.Measurement,
) float64 {
	multiplier := 1.0

	if act.Fraction > 0 {
		multiplier = act.Fraction
	}

	fraction := effectiveFraction(costs) * multiplier

	regime := perspectives.ClassifyRegime(snapshots).Regime
	scale := regimeSizeScale(regime)

	if scale > 0 && scale != 1 {
		fraction *= scale
	}

	if fraction > 1 {
		fraction = 1
	}

	return fraction
}

func regimeSizeScale(regime perspectives.Regime) float64 {
	config := viper.GetViper()

	switch regime {
	case perspectives.RegimeChoppy:
		key := "trading.replay.choppy_size_scale"

		if !config.IsSet(key) {
			errnie.Error(nil, "replay: missing %s, using scale 1.0", key)

			return 1
		}

		return config.GetFloat64(key)
	case perspectives.RegimeBearish:
		key := "trading.replay.bearish_size_scale"

		if !config.IsSet(key) {
			errnie.Error(nil, "replay: missing %s, using scale 1.0", key)

			return 1
		}

		return config.GetFloat64(key)
	default:
		return 1
	}
}
