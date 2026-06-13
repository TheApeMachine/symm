package market

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/config"
)

/*
SignalMeasurementCapacity derives per-signal ring capacity from derived regime
window and current adaptive contagion pacing.
*/
func SignalMeasurementCapacity() (int, error) {
	base, err := config.BaseMeasurementCapacity()

	if err != nil {
		return 0, err
	}

	minObs, minObsErr := config.DerivedMinObs()

	if minObsErr != nil {
		return base, nil
	}

	controller, adaptErr := LoadAdaptation()

	if adaptErr != nil {
		return base, nil
	}

	_, _, slowWindow := controller.ContagionWindows()
	regime, regimeErr := config.DerivedRegimeSpec()

	if regimeErr != nil {
		return base, nil
	}

	slowMin := controller.config.SlowWindowMin
	slowMax := controller.config.SlowWindowMax

	if slowMin <= 0 {
		slowMin = minObs
	}

	window := regime.Window

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
MustSignalMeasurementCapacity resolves signal capacity or panics when derived
regime configuration is invalid.
*/
func MustSignalMeasurementCapacity() int {
	capacity, err := SignalMeasurementCapacity()

	if err != nil {
		panic(fmt.Sprintf("market capacity: %v", err))
	}

	return capacity
}
