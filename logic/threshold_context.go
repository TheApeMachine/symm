package logic

import (
	"math"

	"github.com/spf13/viper"
)

const defaultExitConfidenceFloor = 0.35

/*
ThresholdContext carries runtime playbook thresholds derived from market state.
*/
type ThresholdContext struct {
	ExitConfidenceBaseline float64
}

/*
NewThresholdContext builds exit confidence thresholds from config and regime vol.
*/
func NewThresholdContext(regimeVolatility float64) ThresholdContext {
	entryBaseline := viper.GetFloat64("trading.entry.confidence_baseline")

	if entryBaseline <= 0 {
		entryBaseline = defaultConfidenceBaseline
	}

	exitBaseline := viper.GetFloat64("trading.exit.confidence_baseline")

	if exitBaseline <= 0 {
		exitBaseline = entryBaseline - 0.05
	}

	turbulenceScale := viper.GetFloat64("trading.entry.turbulence_confidence_scale")

	if turbulenceScale > 0 && regimeVolatility > 0 {
		exitBaseline -= turbulenceScale * regimeVolatility
	}

	floor := viper.GetFloat64("trading.exit.confidence_floor")

	if floor <= 0 {
		floor = defaultExitConfidenceFloor
	}

	exitBaseline = math.Max(exitBaseline, floor)

	return ThresholdContext{
		ExitConfidenceBaseline: exitBaseline,
	}
}
