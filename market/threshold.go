package market

import (
	"math"

	"github.com/theapemachine/symm/logic"
)

const (
	// confidenceBaselineFloor keeps the derived spread from collapsing to zero
	// during quiet stretches; it is a numerical guard, not a trading threshold.
	confidenceBaselineFloor = 0.01
	// confidenceBaselineMinObs is the warmup count before the derived bar is
	// trusted; until then the config seed is used.
	confidenceBaselineMinObs = 64
	// confidenceBaselineAlpha is the EWMA blend rate for the confidence stream.
	confidenceBaselineAlpha = 0.05
	// entrySigmaBase / entrySigmaTempScale turn the macro temperature into the
	// number of standard deviations above the live confidence mean a signal must
	// reach to enter. Hot market -> larger sigma -> only the most decisive
	// signals clear the bar. These are derived-runtime knobs, not price magic.
	entrySigmaBase      = 1.0
	entrySigmaTempScale = 2.0
)

func (story *Story) thresholdContextFromMean(mean RegimeStrengths, ready bool) logic.ThresholdContext {
	ctx := logic.NewThresholdContext(
		story.tree.ThresholdConfig(),
		regimeVolatilityFromMean(mean, ready),
		marketTemperatureFromMean(mean, ready),
	)

	if derived, ok := story.derivedEntryBaselineFromTemperature(marketTemperatureFromMean(mean, ready), ready); ok {
		ctx.EntryConfidenceBaseline = derived
	}

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

	sigma := entrySigmaBase + entrySigmaTempScale*temperature

	bar, ok := story.confidenceBaseline.Threshold(sigma)

	if !ok {
		return 0, false
	}

	thresholdConfig := story.tree.ThresholdConfig()
	bar = math.Max(bar, thresholdConfig.EntryConfidenceBaseline)
	bar = math.Min(bar, thresholdConfig.EntryConfidenceCeiling)

	return bar, true
}

func (story *Story) thresholdContext() logic.ThresholdContext {
	ctx := logic.NewThresholdContext(
		story.tree.ThresholdConfig(),
		story.regimeVolatility(),
		story.marketTemperature(),
	)

	if derived, ok := story.derivedEntryBaseline(); ok {
		ctx.EntryConfidenceBaseline = derived
	}

	return ctx
}

/*
observeConfidence folds a signal's confidence into the adaptive baseline so the
entry bar tracks the live distribution of confidences instead of a fixed number.
*/
func (story *Story) observeConfidence(confidence float64) {
	if story == nil || story.confidenceBaseline == nil {
		return
	}

	if confidence <= 0 || math.IsNaN(confidence) || math.IsInf(confidence, 0) {
		return
	}

	_ = story.confidenceBaseline.Observe(confidence, confidenceBaselineAlpha)
}

/*
derivedEntryBaseline returns the adaptive entry confidence bar: the confidence a
signal must exceed is entrySigma standard deviations above the live mean
confidence, where entrySigma rises with the macro market temperature. This is
the magic-free entry gate — "decisive" is defined relative to what the signals
are currently producing, scaled by macro heat, rather than a guessed constant.
Returns false until the baseline has warmed up.
*/
func (story *Story) derivedEntryBaseline() (float64, bool) {
	if story == nil || story.confidenceBaseline == nil || !story.confidenceBaseline.Ready() {
		return 0, false
	}

	temperature := story.marketTemperature()
	sigma := entrySigmaBase + entrySigmaTempScale*temperature

	bar, ok := story.confidenceBaseline.Threshold(sigma)

	if !ok {
		return 0, false
	}

	// Bound into the configured interval so a pathological spread cannot drive
	// the derived bar below the warmup seed or above the ceiling.
	thresholdConfig := story.tree.ThresholdConfig()
	bar = math.Max(bar, thresholdConfig.EntryConfidenceBaseline)
	bar = math.Min(bar, thresholdConfig.EntryConfidenceCeiling)

	return bar, true
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
and choppy the market is on average right now. It is the top of the ontological
hierarchy — a hot market raises the entry confidence bar for every micro signal
(see logic.NewThresholdContext). Both inputs are already 0..1 regime strengths,
so the temperature stays in 0..1.
*/
func (story *Story) marketTemperature() float64 {
	if story == nil || story.regime == nil {
		return 0
	}

	mean, ready := story.regime.MarketMean()

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
