package config

import (
	"fmt"
)

/*
BaseMeasurementCapacity derives the nominal ring capacity from derived regime sizing.
*/
func BaseMeasurementCapacity() (int, error) {
	regime, err := DerivedRegimeSpec()

	if err != nil {
		return 0, err
	}

	window := regime.Window
	minObs := regime.MinSamples

	base := window / 4

	if base < minObs {
		base = minObs
	}

	maxCapacity := window / 2

	if base > maxCapacity {
		base = maxCapacity
	}

	if base <= 0 {
		return 0, fmt.Errorf("config: derived measurement capacity must be positive")
	}

	return base, nil
}

/*
DerivedMinObs returns the warmup observation count for adaptive baselines.
*/
func DerivedMinObs() (int, error) {
	regime, err := DerivedRegimeSpec()

	if err != nil {
		return 0, err
	}

	return regime.MinSamples, nil
}
