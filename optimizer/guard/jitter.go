package guard

import (
	"context"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
)

/*
robustUnderJitter re-scores perturbed threshold copies; rejects brittle gates.
*/
func robustUnderJitter(
	ctx context.Context,
	branches perspectives.BranchList,
	tape replay.ReplayTape,
	fractions []float64,
	baselineScore float64,
	profile *profile.Profile,
) bool {
	if baselineScore <= 0 || len(fractions) == 0 {
		return baselineScore > 0
	}

	floor := baselineScore * 0.5

	for _, fraction := range fractions {
		perturbed := perturbBranchValues(branches, fraction, profile)
		score := replay.NewReplaySimulationWithTape(ctx, perturbed, tape).Result().Score

		if score < floor {
			return false
		}
	}

	return true
}

func perturbBranchValues(
	branches perspectives.BranchList,
	fraction float64,
	profile *profile.Profile,
) perspectives.BranchList {
	perturbed := branches.Clone()

	for index := range perturbed {
		perturbBranchTree(&perturbed[index], fraction, profile)
	}

	return perturbed
}

func perturbBranchTree(
	branch *perspectives.Branch,
	fraction float64,
	profile *profile.Profile,
) {
	if branch.ValueSet {
		branch.Value = perturbBranchValue(
			branch.Value,
			fraction,
			profile.JitterScale(branch.Category, branch.Unit, branch.Value),
		)
	}

	for index := range branch.Branches {
		perturbBranchTree(&branch.Branches[index], fraction, profile)
	}
}

func perturbBranchValue(value float64, fraction float64, scale float64) float64 {
	return value + fraction*scale
}
