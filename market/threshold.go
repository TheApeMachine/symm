package market

import (
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
)

const (
	// confidenceBaselineFloor keeps the derived spread from collapsing to zero
	// during quiet stretches; it is a numerical guard, not a trading threshold.
	confidenceBaselineFloor = 0.01
	// confidenceBaselineMinObs is the warmup count before the derived bar is
	// trusted; until then entries stay blocked.
	confidenceBaselineMinObs = 64
	// confidenceBaselineAlpha is the EWMA blend rate for the confidence stream.
	confidenceBaselineAlpha = 0.05
	// unreachedEntryBar blocks entries until the confidence baseline warms up.
	unreachedEntryBar = 1.0 - confidenceBaselineFloor
)

func (story *Story) thresholdContextFromMean(mean RegimeStrengths, ready bool) logic.ThresholdContext {
	regimeVol := regimeVolatilityFromMean(mean, ready)
	temperature := marketTemperatureFromMean(mean, ready)
	entryBar := unreachedEntryBar

	if derived, ok := story.derivedEntryBaselineFromTemperature(temperature, ready); ok {
		entryBar = derived
	}

	ctx := logic.NewThresholdContext(entryBar, regimeVol, temperature)
	ctx.SurpriseBaseline = WarmupSurpriseThreshold(temperature)
	GlobalSurpriseRegistry().SetTemperature(temperature)

	return ctx
}

func regimeVolatilityFromMean(mean RegimeStrengths, ready bool) float64 {
	if !ready {
		return 0
	}

	return mean.Volatility
}

func marketTemperatureFromMean(mean RegimeStrengths, ready bool) float64 {
	if !ready {
		return 0
	}

	temperature := (mean.Volatility + mean.Choppiness) / 2

	if temperature < 0 {
		return 0
	}

	if temperature > 1 {
		return 1
	}

	return temperature
}

func (story *Story) derivedEntryBaselineFromTemperature(temperature float64, ready bool) (float64, bool) {
	if story == nil || story.confidenceBaseline == nil || !story.confidenceBaseline.Ready() || !ready {
		return 0, false
	}

	sigma, sigmaOk := entrySigmaFromTemperature(temperature)

	if !sigmaOk {
		return 0, false
	}

	bar, ok := story.confidenceBaseline.Threshold(sigma)

	if !ok {
		return 0, false
	}

	return clampEntryBar(bar), true
}

func (story *Story) thresholdContext() logic.ThresholdContext {
	regimeVol := story.regimeVolatility()
	temperature := story.marketTemperature()
	entryBar := unreachedEntryBar

	if derived, ok := story.derivedEntryBaseline(); ok {
		entryBar = derived
	}

	ctx := logic.NewThresholdContext(entryBar, regimeVol, temperature)
	ctx.SurpriseBaseline = WarmupSurpriseThreshold(temperature)
	GlobalSurpriseRegistry().SetTemperature(temperature)

	return ctx
}

/*
observeConfidence folds a signal's confidence into the adaptive baseline so the
entry bar tracks the live distribution of confidences instead of a fixed number.
*/
func (story *Story) observeConfidence(confidence float64, source logic.SourceType) {
	if story == nil || story.confidenceBaseline == nil {
		return
	}

	if confidence <= 0 || math.IsNaN(confidence) || math.IsInf(confidence, 0) {
		return
	}

	switch source {
	case logic.SourceToxicity, logic.SourceLiquidity, logic.SourceExhaustion:
		return
	default:
	}

	_ = story.confidenceBaseline.Observe(confidence, confidenceBaselineAlpha)
}

/*
derivedEntryBaseline returns the adaptive entry confidence bar: the confidence a
signal must exceed is entrySigma standard deviations above the live mean
confidence, where entrySigma rises with the macro market temperature.
Returns false until the baseline has warmed up.
*/
func (story *Story) derivedEntryBaseline() (float64, bool) {
	if story == nil || story.confidenceBaseline == nil || !story.confidenceBaseline.Ready() {
		return 0, false
	}

	temperature := story.marketTemperature()
	sigma, sigmaOk := entrySigmaFromTemperature(temperature)

	if !sigmaOk {
		return 0, false
	}

	bar, ok := story.confidenceBaseline.Threshold(sigma)

	if !ok {
		return 0, false
	}

	return clampEntryBar(bar), true
}

func entrySigmaFromTemperature(temperature float64) (float64, bool) {
	controller, err := LoadAdaptation()

	if err != nil || controller == nil {
		errnie.Debug(fmt.Sprintf(
			"market: LoadAdaptation failed for entry sigma at temperature %.4f: %v",
			temperature,
			err,
		))

		return 0, false
	}

	return controller.TrendSigmaAt(temperature), true
}

func clampEntryBar(bar float64) float64 {
	if bar < confidenceBaselineFloor {
		return confidenceBaselineFloor
	}

	ceiling := 1.0 - confidenceBaselineFloor

	if bar > ceiling {
		return ceiling
	}

	return bar
}

func (story *Story) regimeVolatility() float64 {
	if story == nil || story.regime == nil {
		return 0
	}

	mean, ready := story.regime.MarketMean()

	if !ready {
		return 0
	}

	return mean.Volatility
}

/*
marketTemperature is the macro "heat" of the whole cross-section: how volatile
and choppy the market is on average right now. Both inputs are already 0..1
regime strengths, so the temperature stays in 0..1.
*/
func (story *Story) marketTemperature() float64 {
	if story == nil || story.regime == nil {
		return 0
	}

	mean, ready := story.regime.MarketMean()

	if !ready {
		return 0
	}

	return marketTemperatureFromMean(mean, ready)
}
