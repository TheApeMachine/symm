package logic

import (
	"math"

	"github.com/theapemachine/symm/config"
)

/*
ThresholdContext carries runtime playbook thresholds derived from market state.
*/
type ThresholdContext struct {
	ExitConfidenceBaseline  float64
	EntryConfidenceBaseline float64
}

/*
NewThresholdContext builds the runtime confidence bars from config, regime
volatility and the macro market temperature.

The exit bar drops in turbulence (let losers out faster). The entry bar is the
ontological-hierarchy gate: it RISES with the macro temperature so that when the
whole market is hot/frenzied, a micro signal must be more decisive before it can
open a position — macro context dictates micro sensitivity, instead of every
signal voting on equal footing.
*/
func NewThresholdContext(
	thresholdConfig config.ThresholdConfig,
	regimeVolatility float64,
	marketTemperature float64,
) ThresholdContext {
	exitBaseline := thresholdConfig.ExitConfidenceBaseline
	turbulenceScale := thresholdConfig.TurbulenceConfidenceScale

	if turbulenceScale > 0 && regimeVolatility > 0 {
		exitBaseline -= turbulenceScale * regimeVolatility
	}

	exitBaseline = math.Max(exitBaseline, thresholdConfig.ExitConfidenceFloor)

	entryBaseline := thresholdConfig.EntryConfidenceBaseline
	temperatureScale := thresholdConfig.EntryTemperatureScale

	if temperatureScale > 0 && marketTemperature > 0 {
		entryBaseline += temperatureScale * marketTemperature
	}

	if thresholdConfig.EntryConfidenceCeiling > 0 {
		entryBaseline = math.Min(entryBaseline, thresholdConfig.EntryConfidenceCeiling)
	}

	return ThresholdContext{
		ExitConfidenceBaseline:  exitBaseline,
		EntryConfidenceBaseline: entryBaseline,
	}
}
