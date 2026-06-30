package resonance

import (
	"math"

	"github.com/theapemachine/symm/logic"
)

/*
MeasureTargets maps a resonance attention mode to the specialist signals the
trader should Measure when that mode dominates.
*/
func MeasureTargets(category logic.CategoryType) []string {
	switch string(category) {
	case CategoryFlow:
		return []string{
			"fluid",
			"depthflow",
			"exhaust",
			"liquidity",
		}
	case CategoryStress:
		return []string{
			"toxicity",
			"hawkes",
			"pumpdump",
			"cvd",
		}
	case CategoryCoupling:
		return []string{
			"correlation",
			"leadlag",
			"causal",
			"sentiment",
			"manifold",
		}
	default:
		return nil
	}
}

/*
AttentionCategoryIndex maps batch-settle latent modes to resonance attention modes.
Spread at or below zero routes to equilibrium regardless of latent state.
Stress only wins when the stress component carries more activation than the
nonstress modes combined; otherwise a merely-largest cold-start axis would turn
baseline flow into a false turbulent label.
*/
func AttentionCategoryIndex(spread float64, latent []float64) int {
	if spread <= 0 {
		return 3
	}

	if len(latent) == 0 {
		return 1
	}

	stressActivation := 0.0
	nonstressActivation := 0.0

	for index, value := range latent {
		activation := math.Abs(value)

		if index == 1 {
			stressActivation += activation
		} else {
			nonstressActivation += activation
		}
	}

	if stressActivation > nonstressActivation {
		return 2
	}

	return 1
}

func attentionStrength(
	categoryIndex int,
	spread, surprise float64,
	latent []float64,
) float64 {
	flowScore := 0.0
	stressScore := 0.0
	couplingScore := 0.0
	peakActivation := 0.0

	for _, value := range latent {
		peakActivation = math.Max(peakActivation, math.Abs(value))
	}

	switch categoryIndex {
	case 1:
		flowScore = math.Max(peakActivation, 1/(1+surprise))
	case 2:
		stressScore = math.Max(peakActivation, 1/(1+surprise))
	case 3:
		couplingScore = math.Max(math.Abs(spread), 1/(1+surprise))
	}

	switch categoryIndex {
	case 1:
		return flowScore
	case 2:
		return stressScore
	case 3:
		return couplingScore
	}

	if peakActivation <= 0 {
		return 1 / (1 + surprise)
	}

	return peakActivation
}

/*
AttentionConfidence derives publishable attention strength from settle surprise and latent state.
*/
func AttentionConfidence(
	spread, surprise float64,
	latent []float64,
) float64 {
	categoryIndex := AttentionCategoryIndex(spread, latent)
	strength := attentionStrength(categoryIndex, spread, surprise, latent)

	if math.IsNaN(strength) || math.IsInf(strength, 0) || strength <= 0 {
		return 0
	}

	return strength
}
