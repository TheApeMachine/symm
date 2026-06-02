package optimizer

import (
	"context"

	"github.com/theapemachine/symm/market/perspectives"
)

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
	if branch.ValueSet && branch.Value != 0 {
		branch.Value *= 1 + fraction
	}

	for index := range branch.Branches {
		perturbBranchTree(&branch.Branches[index], fraction)
	}
}
