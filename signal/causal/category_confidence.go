package causal

import (
	"math"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
)

const confoundFraction = 0.25

/*
causalOutcome is the Pearl-ladder read plus the margins that separate categories.
*/
type causalOutcome struct {
	raw          float64
	reason       string
	intervention float64
	association  float64
	uplift       float64
	inverted     bool
	contagion    float64
	condition    float64
}

/*
categoryConfidence returns how decisively the assigned category wins over its
neighbors — not how large the strength is. A clear CausalNoise read can score
high; a borderline Alpha/Shock flip reads low.
*/
func categoryConfidence(
	category perspectives.CategoryType,
	outcome causalOutcome,
	macroMomentum, changePct, buyPressure float64,
	onLadder bool,
) float64 {
	if onLadder {
		return ladderCategoryConfidence(category, outcome)
	}

	return fallbackCategoryConfidence(category, macroMomentum, changePct, buyPressure)
}

func ladderCategoryConfidence(
	category perspectives.CategoryType,
	outcome causalOutcome,
) float64 {
	scale := math.Max(
		math.Abs(outcome.intervention),
		math.Max(math.Abs(outcome.association), 1e-12),
	)

	interventionMargin := outcome.intervention / scale

	if interventionMargin <= 0 {
		return 0
	}

	switch category {
	case perspectives.CategoryLiquidityShock:
		return math.Min(interventionMargin, inversionMarginAbove(outcome))
	case perspectives.CategoryEndogenousAlpha:
		return math.Min(interventionMargin, inversionMarginBelow(outcome))
	default:
		return 0
	}
}

func fallbackCategoryConfidence(
	category perspectives.CategoryType,
	macroMomentum, changePct, buyPressure float64,
) float64 {
	switch category {
	case perspectives.CategorySystemicBeta:
		return fallbackBetaConfidence(macroMomentum, changePct, buyPressure)
	case perspectives.CategoryCausalNoise:
		return fallbackNoiseConfidence(macroMomentum, changePct, buyPressure)
	default:
		return 0
	}
}

func fallbackBetaConfidence(macroMomentum, changePct, buyPressure float64) float64 {
	if buyPressure != 0 && changePct == 0 {
		return 0
	}

	scale := math.Max(
		math.Abs(macroMomentum),
		math.Max(math.Abs(changePct), 1e-12),
	)

	return math.Max(math.Abs(macroMomentum), math.Abs(changePct)) / scale
}

func fallbackNoiseConfidence(macroMomentum, changePct, buyPressure float64) float64 {
	if buyPressure == 0 || changePct != 0 {
		return 0
	}

	scale := math.Max(
		math.Abs(buyPressure),
		math.Max(math.Abs(macroMomentum), 1e-12),
	)

	return math.Abs(buyPressure) / scale
}

func inversionMarginBelow(outcome causalOutcome) float64 {
	margin := math.MaxFloat64

	if config.System.CausalContagionBreak > 0 {
		headroom := config.System.CausalContagionBreak - outcome.contagion

		if headroom < margin {
			margin = headroom / config.System.CausalContagionBreak
		}
	}

	if config.System.CausalConditionSwitch > 0 && outcome.condition > 0 &&
		!math.IsInf(outcome.condition, 1) {
		headroom := config.System.CausalConditionSwitch - outcome.condition

		if headroom < margin {
			margin = headroom / config.System.CausalConditionSwitch
		}
	}

	if margin == math.MaxFloat64 {
		return 1
	}

	if margin <= 0 {
		return 0
	}

	return margin
}

func inversionMarginAbove(outcome causalOutcome) float64 {
	margin := -1.0

	if config.System.CausalContagionBreak > 0 &&
		outcome.contagion >= config.System.CausalContagionBreak {
		excess := outcome.contagion - config.System.CausalContagionBreak
		span := 1 - config.System.CausalContagionBreak

		if span > 0 {
			margin = math.Max(margin, excess/span)
		}
	}

	if config.System.CausalConditionSwitch > 0 &&
		(math.IsInf(outcome.condition, 1) ||
			outcome.condition >= config.System.CausalConditionSwitch) {
		if math.IsInf(outcome.condition, 1) {
			margin = math.Max(margin, 1)
		} else {
			excess := outcome.condition - config.System.CausalConditionSwitch
			margin = math.Max(margin, excess/config.System.CausalConditionSwitch)
		}
	}

	if margin < 0 {
		return 0
	}

	return margin
}
