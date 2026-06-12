package logic

import (
	"math"

	"github.com/theapemachine/symm/config"
)

/*
ThresholdContext carries runtime playbook thresholds derived from market state.
*/
type ThresholdContext struct {
	ExitConfidenceBaseline float64
}

/*
NewThresholdContext builds exit confidence thresholds from config and regime vol.
*/
func NewThresholdContext(
	thresholdConfig config.ThresholdConfig,
	regimeVolatility float64,
) ThresholdContext {
	exitBaseline := thresholdConfig.ExitConfidenceBaseline
	turbulenceScale := thresholdConfig.TurbulenceConfidenceScale

	if turbulenceScale > 0 && regimeVolatility > 0 {
		exitBaseline -= turbulenceScale * regimeVolatility
	}

	exitBaseline = math.Max(exitBaseline, thresholdConfig.ExitConfidenceFloor)

	return ThresholdContext{
		ExitConfidenceBaseline: exitBaseline,
	}
}
