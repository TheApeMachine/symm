package config

import (
	"fmt"

	"github.com/spf13/viper"
)

/*
BaseMeasurementCapacity derives the nominal ring capacity from regime window
settings without consulting live adaptation state.
*/
func BaseMeasurementCapacity() (int, error) {
	window := viper.GetInt("regime.window")
	minObs := viper.GetInt("regime.baseline.min_obs")

	if window <= 0 {
		return 0, fmt.Errorf("config: regime.window must be positive")
	}

	if minObs <= 0 {
		return 0, fmt.Errorf("config: regime.baseline.min_obs must be positive")
	}

	base := window / 4

	if base < minObs {
		base = minObs
	}

	maxCapacity := window / 2

	if base > maxCapacity {
		base = maxCapacity
	}

	return base, nil
}
