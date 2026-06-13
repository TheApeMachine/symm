package signal

import (
	"errors"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market"
)

var errInvalidDerivedInterval = errors.New("signal: derived integration interval must be positive")

/*
ClassifierAlpha returns the live EWMA blend rate shared by transition matrices.
*/
func ClassifierAlpha() float64 {
	controller, err := market.LoadAdaptation()

	if err != nil || controller == nil {
		regime, regimeErr := config.DerivedRegimeSpec()

		if regimeErr != nil {
			return 1.0 / 64.0
		}

		baseline := config.DerivedBaselineSpec(regime)

		return baseline.AlphaMin
	}

	return controller.Alpha()
}

/*
ScoreInertiaEffort derives how many consecutive readings must agree before a score moves.
*/
func ScoreInertiaEffort() int {
	regime, err := config.DerivedRegimeSpec()

	if err != nil {
		return 1
	}

	return regime.MinSamples
}

/*
VolumeClockBarsPerDay estimates gauge frames per day from publish cadence.
*/
func VolumeClockBarsPerDay() float64 {
	return config.DerivedVolumeClockBarsPerDay()
}

/*
BoundedClassifierAlpha clamps the adaptive alpha into the classifier operating range.
*/
func BoundedClassifierAlpha() float64 {
	alpha := ClassifierAlpha()

	if alpha < 0.1 {
		return 0.1
	}

	if alpha > 1.0 {
		return 1.0
	}

	return alpha
}

/*
DerivedGridHalfWidth scales lattice half-width from subscribed book depth.
*/
func DerivedGridHalfWidth(scale int) (int, error) {
	depth, err := config.DerivedBookDepthLevels()

	if err != nil {
		return 0, err
	}

	halfWidth := depth * scale

	if halfWidth < 1 {
		halfWidth = 1
	}

	return halfWidth, nil
}

/*
DerivedIntegrationInterval aligns heavy compute steps with the gauge cadence.
*/
func DerivedIntegrationInterval(multiplier int) (time.Duration, error) {
	if multiplier < 1 {
		multiplier = 1
	}

	interval := config.DerivedPublishInterval() * time.Duration(multiplier)

	if interval <= 0 {
		return 0, errInvalidDerivedInterval
	}

	return interval, nil
}
