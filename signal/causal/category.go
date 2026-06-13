package causal

import (
	"math"

	"github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
)

// uniformCausalConfidence is the 1/N floor across the four causal categories
// (endogenous alpha, liquidity shock, systemic beta, causal noise): a read with no
// separating margin is no better than a uniform guess, and is never zero confidence.
const uniformCausalConfidence = 1.0 / 4.0

/*
causalCategory maps the causal reason onto the structural-origin perspective.
*/
func causalCategory(reason string) logic.CategoryType {
	switch reason {
	case "intervention", "counterfactual_like":
		return logic.CategoryEndogenousAlpha
	case "intervention_regime_inversion", "counterfactual_like_regime_inversion":
		return logic.CategoryLiquidityShock
	case "macro_association":
		return logic.CategorySystemicBeta
	default:
		return logic.CategoryCausalNoise
	}
}

/*
causalCategoryScores returns non-negative evidence for each causal category slot.
*/
func causalCategoryScores(
	outcome causal.Outcome,
	macroMomentum, changePct, buyPressure float64,
	onLadder bool,
) []float64 {
	alphaScore := 0.0
	shockScore := 0.0

	if onLadder {
		alphaScore = ladderEvidence(logic.CategoryEndogenousAlpha, outcome)
		shockScore = ladderEvidence(logic.CategoryLiquidityShock, outcome)
	}

	return []float64{
		alphaScore,
		shockScore,
		betaEvidence(macroMomentum, changePct, buyPressure),
		noiseEvidence(macroMomentum, changePct, buyPressure),
	}
}

/*
causalShareConfidence returns the selected category's share of total causal evidence.
*/
func causalShareConfidence(
	category logic.CategoryType,
	outcome causal.Outcome,
	macroMomentum, changePct, buyPressure float64,
	onLadder bool,
) (float64, error) {
	confidence, err := probability.CategoryShareConfidence(
		causalCategoryScores(outcome, macroMomentum, changePct, buyPressure, onLadder),
		causalCategoryIndex(category),
	)

	if err != nil {
		return 0, err
	}

	if confidence <= 0 {
		return uniformCausalConfidence, nil
	}

	return confidence, nil
}

func causalCategoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategoryEndogenousAlpha:
		return 1
	case logic.CategoryLiquidityShock:
		return 2
	case logic.CategorySystemicBeta:
		return 3
	case logic.CategoryCausalNoise:
		return 4
	default:
		return 0
	}
}

/*
causalEvidence returns the category-selection clarity for the assigned category —
how decisively the ladder (or association) read separates it from the neighbouring
categories. It is distinct from the SNR standout, which carries the raw magnitude of
the causal effect.
*/
func causalEvidence(
	category logic.CategoryType,
	outcome causal.Outcome,
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

func ladderEvidence(category logic.CategoryType, outcome causal.Outcome) float64 {
	interventionMargin := ladderInterventionEvidence(outcome)

	if interventionMargin <= 0 {
		return 0
	}

	switch category {
	case logic.CategoryLiquidityShock:
		return math.Min(interventionMargin, inversionMarginAbove(outcome))
	case logic.CategoryEndogenousAlpha:
		return math.Min(interventionMargin, inversionMarginBelow(outcome))
	default:
		return 0
	}
}

func associationEvidence(
	category logic.CategoryType,
	macroMomentum, changePct, buyPressure float64,
) float64 {
	switch category {
	case logic.CategorySystemicBeta:
		return betaEvidence(macroMomentum, changePct, buyPressure)
	case logic.CategoryCausalNoise:
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

	return alignment * magnitudeMargin(shared)
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

	return alignment * magnitudeMargin(flow)
}

func ladderInterventionEvidence(outcome causal.Outcome) float64 {
	intervention := math.Abs(outcome.Intervention)

	if intervention <= 0 {
		return 0
	}

	runner := math.Max(math.Abs(outcome.Association), math.Abs(outcome.Uplift))
	alignment := 1.0

	if runner > 0 {
		alignment = intervention / (intervention + runner)
	}

	return alignment * magnitudeMargin(intervention)
}

func competitionMargin(excess, span float64) float64 {
	if excess <= 0 || span <= 0 {
		return 0
	}

	return excess / (excess + span)
}

func magnitudeMargin(value float64) float64 {
	if value <= 0 {
		return 0
	}

	return value / (1 + value)
}

func inversionMarginBelow(outcome causal.Outcome) float64 {
	config := loadRuntimeConfig()
	contagionBreak := config.ContagionBreak
	conditionSwitch := config.ConditionSwitch
	margin := math.MaxFloat64

	if contagionBreak > 0 {
		headroom := contagionBreak - outcome.Contagion

		if headroom < margin {
			margin = competitionMargin(headroom, contagionBreak)
		}
	}

	if conditionSwitch > 0 && outcome.Condition > 0 &&
		!math.IsInf(outcome.Condition, 1) {
		headroom := conditionSwitch - outcome.Condition

		if headroom < margin {
			span := conditionSwitch

			if headroom <= 0 {
				margin = 0
			} else {
				margin = competitionMargin(headroom, span)
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

func inversionMarginAbove(outcome causal.Outcome) float64 {
	config := loadRuntimeConfig()
	contagionBreak := config.ContagionBreak
	conditionSwitch := config.ConditionSwitch
	margin := -1.0

	if contagionBreak > 0 &&
		outcome.Contagion >= contagionBreak {
		excess := outcome.Contagion - contagionBreak
		span := 1 - contagionBreak

		if span > 0 {
			margin = math.Max(margin, competitionMargin(excess, span))
		}
	}

	if conditionSwitch > 0 &&
		(math.IsInf(outcome.Condition, 1) ||
			outcome.Condition >= conditionSwitch) {
		if math.IsInf(outcome.Condition, 1) {
			margin = math.Max(margin, magnitudeMargin(math.Abs(outcome.Intervention)))
		} else {
			excess := outcome.Condition - conditionSwitch
			score := competitionMargin(excess, conditionSwitch)

			margin = math.Max(margin, score)
		}
	}

	if margin < 0 {
		return 0
	}

	return margin
}
