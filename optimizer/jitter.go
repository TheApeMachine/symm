package optimizer

import (
	"context"
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

const jitterReferenceMagnitude = 1.0

/*
robustUnderJitter re-scores perturbed threshold copies; rejects brittle gates.
*/
func robustUnderJitter(
	ctx context.Context,
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
	fractions []float64,
	baselineScore float64,
) bool {
	if baselineScore <= 0 || len(fractions) == 0 {
		return baselineScore > 0
	}

	floor := baselineScore * 0.5

	for _, fraction := range fractions {
		perturbed := perturbBranchValues(branches, fraction)
		score := NewReplaySimulation(ctx, perturbed, rows).Result().Score

		if score < floor {
			return false
		}
	}

	return true
}

func perturbBranchValues(
	branches perspectives.BranchList, fraction float64,
) perspectives.BranchList {
	perturbed := branches.Clone()

	for index := range perturbed {
		perturbBranchTree(&perturbed[index], fraction)
	}

	return perturbed
}

func perturbBranchTree(branch *perspectives.Branch, fraction float64) {
	if branch.ValueSet {
		branch.Value = perturbBranchValue(branch.Value, fraction)
	}

	for index := range branch.Branches {
		perturbBranchTree(&branch.Branches[index], fraction)
	}
}

func perturbBranchValue(value float64, fraction float64) float64 {
	magnitude := math.Max(math.Abs(value), jitterReferenceMagnitude)

	return value + fraction*magnitude
}
