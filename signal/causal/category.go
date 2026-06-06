package causal

import (
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives/types"
)

const confoundFraction = 0.25

// uniformCausalConfidence is the 1/N floor across the four causal categories
// (endogenous alpha, liquidity shock, systemic beta, causal noise): a read with no
// separating margin is no better than a uniform guess, and is never zero confidence.
const uniformCausalConfidence = 1.0 / 4.0

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
causalCategory maps the causal reason onto the structural-origin perspective.
*/
func causalCategory(reason string) types.CategoryType {
	switch reason {
	case "intervention", "counterfactual_like":
		return types.CategoryEndogenousAlpha
	case "intervention_regime_inversion", "counterfactual_like_regime_inversion":
		return types.CategoryLiquidityShock
	case "macro_association":
		return types.CategorySystemicBeta
	default:
		return types.CategoryCausalNoise
	}
}

/*
causalEvidence returns the category-selection clarity for the assigned category —
how decisively the ladder (or association) read separates it from the neighbouring
categories. It is distinct from the SNR standout, which carries the raw magnitude of
the causal effect.
*/
func causalEvidence(
	category types.CategoryType,
	outcome causalOutcome,
	macroMomentum, changePct, buyPressure float64,
	onLadder bool,
) float64 {
	evidence := associationEvidence(category, macroMomentum, changePct, buyPressure)

	if onLadder {
		evidence = ladderEvidence(category, outcome)
	}

	// A causal read with no separating margin is no better than a uniform guess among
	// the categories; confidence floors at 1/N, never 0.
	return math.Max(evidence, uniformCausalConfidence)
}

func ladderEvidence(category types.CategoryType, outcome causalOutcome) float64 {
	interventionMargin := ladderInterventionEvidence(outcome)

	if interventionMargin <= 0 {
		return 0
	}

	switch category {
	case types.CategoryLiquidityShock:
		return math.Min(interventionMargin, inversionMarginAbove(outcome))
	case types.CategoryEndogenousAlpha:
		return math.Min(interventionMargin, inversionMarginBelow(outcome))
	default:
		return 0
	}
}

func associationEvidence(
	category types.CategoryType,
	macroMomentum, changePct, buyPressure float64,
) float64 {
	switch category {
	case types.CategorySystemicBeta:
		return betaEvidence(macroMomentum, changePct, buyPressure)
	case types.CategoryCausalNoise:
		return noiseEvidence(macroMomentum, changePct, buyPressure)
	default:
		return 0
	}
}

func betaEvidence(macroMomentum, changePct, buyPressure float64) float64 {
	if buyPressure != 0 && changePct == 0 {
		return 0
	}

	macro := math.Abs(macroMomentum)
	change := math.Abs(changePct)
	shared := math.Min(macro, change)

	if shared <= 0 {
		return 0
	}

	gap := math.Abs(macro - change)
	alignment := 1.0

	if gap > 0 {
		alignment = shared / (shared + gap)
	}

	return alignment * types.UnitMagnitudeMargin(shared)
}

func noiseEvidence(macroMomentum, changePct, buyPressure float64) float64 {
	if buyPressure == 0 || changePct != 0 {
		return 0
	}

	flow := math.Abs(buyPressure)
	macro := math.Abs(macroMomentum)
	alignment := 1.0

	if macro > 0 {
		alignment = flow / (flow + macro)
	}

	return alignment * types.UnitMagnitudeMargin(flow)
}

func ladderInterventionEvidence(outcome causalOutcome) float64 {
	intervention := math.Abs(outcome.intervention)

	if intervention <= 0 {
		return 0
	}

	runner := math.Max(math.Abs(outcome.association), math.Abs(outcome.uplift))
	alignment := 1.0

	if runner > 0 {
		alignment = intervention / (intervention + runner)
	}

	return alignment * types.UnitMagnitudeMargin(intervention)
}

func inversionMarginBelow(outcome causalOutcome) float64 {
	v := viper.GetViper()
	contagionBreak := v.GetFloat64("signals.causal.contagion_break")
	conditionSwitch := v.GetFloat64("signals.causal.condition_switch")
	margin := math.MaxFloat64

	if contagionBreak > 0 {
		headroom := contagionBreak - outcome.contagion

		if headroom < margin {
			margin = types.UnitCompetitionMargin(headroom, contagionBreak)
		}
	}

	if conditionSwitch > 0 && outcome.condition > 0 &&
		!math.IsInf(outcome.condition, 1) {
		headroom := conditionSwitch - outcome.condition

		if headroom < margin {
			span := conditionSwitch

			if headroom <= 0 {
				margin = 0
			} else {
				margin = types.UnitCompetitionMargin(headroom, span)
			}
		}
	}

	if margin == math.MaxFloat64 {
		return 0
	}

	if margin <= 0 {
		return 0
	}

	return margin
}

func inversionMarginAbove(outcome causalOutcome) float64 {
	v := viper.GetViper()
	contagionBreak := v.GetFloat64("signals.causal.contagion_break")
	conditionSwitch := v.GetFloat64("signals.causal.condition_switch")
	margin := -1.0

	if contagionBreak > 0 &&
		outcome.contagion >= contagionBreak {
		excess := outcome.contagion - contagionBreak
		span := 1 - contagionBreak

		if span > 0 {
			margin = math.Max(margin, types.UnitCompetitionMargin(excess, span))
		}
	}

	if conditionSwitch > 0 &&
		(math.IsInf(outcome.condition, 1) ||
			outcome.condition >= conditionSwitch) {
		if math.IsInf(outcome.condition, 1) {
			margin = math.Max(margin, types.UnitMagnitudeMargin(math.Abs(outcome.intervention)))
		} else {
			excess := outcome.condition - conditionSwitch
			score := types.UnitCompetitionMargin(excess, conditionSwitch)

			margin = math.Max(margin, score)
		}
	}

	if margin < 0 {
		return 0
	}

	return margin
}
