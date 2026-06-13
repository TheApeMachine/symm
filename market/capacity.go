package market

import (
	"fmt"
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/config"
)

/*
SignalMeasurementCapacity derives per-signal ring capacity from the regime
window and current adaptive contagion pacing.
*/
func SignalMeasurementCapacity() (int, error) {
	base, err := config.BaseMeasurementCapacity()

	if err != nil {
		return 0, err
	}

	minObs := viper.GetInt("regime.baseline.min_obs")
	controller, adaptErr := LoadAdaptation()

	if adaptErr != nil {
		return base, nil
	}

	_, _, slowWindow := controller.ContagionWindows()
	slowMin := viper.GetInt("signals.causal.contagion_window_slow_min")
	slowMax := viper.GetInt("signals.causal.contagion_window_slow_max")

	if slowMin <= 0 {
		slowMin = minObs
	}

	window := viper.GetInt("regime.window")

	if slowMax <= 0 {
		slowMax = window / 2
	}

	nominalSlow := (slowMin + slowMax) / 2

	if nominalSlow <= 0 {
		return base, nil
	}

	scaled := int(math.Round(float64(base) * float64(slowWindow) / float64(nominalSlow)))

	if scaled < minObs {
		scaled = minObs
	}

	maxCapacity := window / 2

	if scaled > maxCapacity {
		scaled = maxCapacity
	}

	return scaled, nil
}

/*
MustSignalMeasurementCapacity resolves signal capacity or panics when regime
configuration is invalid.
*/
func MustSignalMeasurementCapacity() int {
	capacity, err := SignalMeasurementCapacity()

	if err != nil {
		panic(fmt.Sprintf("market capacity: %v", err))
	}

	return capacity
}
