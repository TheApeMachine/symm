package logic

import "math"

const exitConfidenceFloor = 0.01

/*
ThresholdContext carries runtime playbook thresholds derived from market state.
*/
type ThresholdContext struct {
	ExitConfidenceBaseline  float64
	EntryConfidenceBaseline float64
	RiskTemperature         float64
	TrendTemperature        float64
	SurpriseBaseline        float64
}

/*
NewThresholdContext builds runtime confidence bars from the live entry baseline,
regime volatility, and macro temperature. Exit relief widens in turbulence so
losers can be cut faster without static playbook constants.
*/
func NewThresholdContext(
	entryConfidenceBaseline float64,
	regimeVolatility float64,
	marketTemperature float64,
) ThresholdContext {
	return ThresholdContext{
		ExitConfidenceBaseline:  deriveExitBaseline(entryConfidenceBaseline, regimeVolatility),
		EntryConfidenceBaseline: entryConfidenceBaseline,
		RiskTemperature:         marketTemperature,
		TrendTemperature:        math.Max(0, marketTemperature-regimeVolatility),
	}
}

func deriveExitBaseline(entryBar, regimeVol float64) float64 {
	if entryBar <= exitConfidenceFloor {
		return exitConfidenceFloor
	}

	turbulence := regimeVol

	if turbulence < 0 {
		turbulence = 0
	}

	if turbulence > 1 {
		turbulence = 1
	}

	relief := turbulence * (entryBar - exitConfidenceFloor)

	return math.Max(exitConfidenceFloor, entryBar-relief)
}

func (ctx ThresholdContext) DynamicSurpriseBaseline() float64 {
	if ctx.SurpriseBaseline > 0 {
		return ctx.SurpriseBaseline
	}

	return 1.0 + 2.0*ctx.RiskTemperature
}
