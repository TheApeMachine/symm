package optimizer

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

const (
	extremePassRateHigh  = 0.95
	extremePassRateLow   = 0.05
	informationGainWaive = 0.75
)

/*
adaptiveComplexityPenalty scales the flat per-step tax by each gate's empirical
selectivity. Informative gates (balanced pass rates) are waived; extreme pass
rates that fingerprint noise are penalized aggressively.
*/
func (guard *OverfitGuard) adaptiveComplexityPenalty(
	branches perspectives.BranchList,
) float64 {
	base := guard.options.ComplexityPenalty

	if base <= 0 {
		return 0
	}

	if guard.profile == nil {
		depth := reasoningDepth(branches)

		return float64(depth) * base
	}

	return sumGateComplexityPenalty(guard.profile, branches, base)
}

func sumGateComplexityPenalty(
	profile *Profile,
	branches perspectives.BranchList,
	basePenalty float64,
) float64 {
	total := 0.0

	for _, branch := range branches {
		total += gateComplexityPenalty(profile, branch, basePenalty)
	}

	return total
}

func gateComplexityPenalty(
	profile *Profile,
	branch perspectives.Branch,
	basePenalty float64,
) float64 {
	penalty := 0.0

	if branch.ValueSet {
		weight := gateComplexityWeight(profile, branch)

		if weight > 0 {
			penalty += basePenalty * weight
		}
	}

	for _, child := range branch.Branches {
		penalty += gateComplexityPenalty(profile, child, basePenalty)
	}

	return penalty
}

func gateComplexityWeight(
	profile *Profile,
	branch perspectives.Branch,
) float64 {
	passes := profile.GatePassCount(
		branch.Category, branch.Unit, branch.Condition, branch.Value,
	)
	categoryTotal := profile.categoryCount(branch.Category)

	if categoryTotal <= 0 {
		return 1
	}

	passRate := float64(passes) / float64(categoryTotal)
	selectivity := gateSelectivity(passRate)

	if selectivity >= informationGainWaive {
		return 0
	}

	if passRate >= extremePassRateHigh || passRate <= extremePassRateLow {
		extremeWeight := 1 + (1 - selectivity)

		return math.Max(extremeWeight, 1)
	}

	if selectivity <= 0 {
		return 1
	}

	return 1 - selectivity
}
