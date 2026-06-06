package perspectives

import (
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func regimeSizeScaleKey(regime types.Regime) (string, error) {
	switch regime {
	case types.RegimeNone:
		return "trading.replay.none_size_scale", nil
	case types.RegimeDead:
		return "trading.replay.dead_size_scale", nil
	case types.RegimeChoppy:
		return "trading.replay.choppy_size_scale", nil
	case types.RegimeTrending:
		return "trading.replay.trending_size_scale", nil
	case types.RegimeBullish:
		return "trading.replay.bullish_size_scale", nil
	case types.RegimeBearish:
		return "trading.replay.bearish_size_scale", nil
	default:
		return "", errnie.Error(fmt.Errorf("perspectives: unknown regime %v", regime))
	}
}

/*
RegimeSizeScale returns the configured entry-size multiplier for regime.
*/
func RegimeSizeScale(regime types.Regime) (float64, error) {
	key, err := regimeSizeScaleKey(regime)

	if err != nil {
		return 0, err
	}

	if !viper.IsSet(key) {
		return 0, errnie.Error(fmt.Errorf("perspectives: %q not configured", key))
	}

	value := viper.GetFloat64(key)

	if value <= 0 {
		return 0, errnie.Error(fmt.Errorf("perspectives: %q must be positive, got %v", key, value))
	}

	return value, nil
}
