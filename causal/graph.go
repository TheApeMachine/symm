package causal

import (
	"math"
	"time"
)

/*
GraphSnapshot publishes the Pearl-ladder DAG state for one symbol without mutating history.
MacroMomentum → PriceVelocity ← LocalFlow, Liquidity as backdoor confounder.
*/
func (state *CausalSymbol) GraphSnapshot(macroMomentum float64, now time.Time) map[string]any {
	batchVolume := state.volumeWindow.Sum()
	localFlow := 0.0

	if batchVolume > 0 && state.buyPressure > 0 {
		localFlow = batchVolume * (state.buyPressure + 1) / 2
	}

	liquidity := bookLiquidity(state.spreadBPS, batchVolume)
	velocity := 0.0

	if len(state.samples) > 0 {
		velocity = state.samples[len(state.samples)-1].priceVelocity
	}

	current := causalSample{
		macroMomentum: macroMomentum,
		liquidity:     liquidity,
		localFlow:     localFlow,
		priceVelocity: velocity,
	}

	sampleCount := len(state.samples)
	ready := sampleCount >= minCausalHistory

	association, intervention, uplift := 0.0, 0.0, 0.0
	confidence := 0.0
	reason := ""
	coefMacro, coefLiquidity, coefFlow := 0.0, 0.0, 0.0

	if ready {
		samples := state.samples
		association = associationEffect(samples)
		intervention = kernelBackdoorFlowEffect(samples) * state.calibrator.Scale()

		if coef, ok := fitStructural(samples); ok {
			coefMacro = coef.macro
			coefLiquidity = coef.liquidity
			coefFlow = coef.flow
		}

		if model, ok := fitNonLinearStructural(samples); ok {
			uplift = nonLinearCounterfactualUplift(current, model, flowInterventionLevel(samples))
		}

		confidence, reason = state.peekScores(current)
	}

	confoundingGap := 0.0

	if ready {
		confoundingGap = math.Abs(intervention - association)
	}

	return map[string]any{
		"event":           "causal_graph",
		"ready":           ready,
		"sample_count":    sampleCount,
		"macro_momentum":  macroMomentum,
		"local_flow":      localFlow,
		"liquidity":       liquidity,
		"price_velocity":  velocity,
		"association":     association,
		"intervention":    intervention,
		"uplift":          uplift,
		"confounding_gap": confoundingGap,
		"confidence":      confidence,
		"reason":          reason,
		"coef_macro":      coefMacro,
		"coef_liquidity":  coefLiquidity,
		"coef_flow":       coefFlow,
	}
}

func (state *CausalSymbol) peekScores(current causalSample) (float64, string) {
	samples := state.samples

	if len(samples) < minCausalHistory {
		return 0, ""
	}

	association := associationEffect(samples)
	intervention := kernelBackdoorFlowEffect(samples) * state.calibrator.Scale()

	if intervention <= 0 {
		return 0, ""
	}

	model, fitOK := fitNonLinearStructural(samples)

	if !fitOK {
		return state.calibrator.NormalizeConfidence(intervention, state.confidenceHistory), "intervention"
	}

	interventionFlow := flowInterventionLevel(samples)
	uplift := nonLinearCounterfactualUplift(current, model, interventionFlow)

	if uplift <= 0 {
		return state.calibrator.NormalizeConfidence(intervention, state.confidenceHistory), "intervention"
	}

	confounded := math.Abs(intervention-association) > math.Abs(association)*0.25
	reason := "intervention"

	if confounded && uplift > 0 {
		reason = "counterfactual_like"
	}

	interventionScore := state.calibrator.NormalizeConfidence(intervention, state.interventionHist)
	upliftScore := state.calibrator.NormalizeConfidence(uplift, state.upliftHist)
	score := interventionScore

	if upliftScore > 0 {
		score = 0.6*interventionScore + 0.4*upliftScore
	}

	if score <= 0 {
		if intervention > 0 {
			return intervention, reason
		}

		return 0, ""
	}

	return score, reason
}
