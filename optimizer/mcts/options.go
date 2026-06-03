package mcts

import (
	"runtime"

	"github.com/theapemachine/symm/optimizer/budget"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
Options bounds deep TreeSearch after shallow beam seeding.
*/
type Options struct {
	Iterations        int
	SeedPriorVisits   int
	MaxReasoningSteps int
	MaxThresholds     int
	Budget            types.SearchBudget
}

func normalizeOptions(
	options Options,
	profile *profile.Profile,
	tape replay.ReplayTape,
	workers int,
) Options {
	searchBudget := options.Budget

	if searchBudget.IsZero() && profile != nil {
		searchBudget = budget.DeriveSearchBudget(profile, tape, workers)
	}

	return ApplyBudget(options, searchBudget)
}

/*
ApplyBudget fills unset MCTS limits from a derived search budget.
*/
func ApplyBudget(options Options, budget types.SearchBudget) Options {
	if options.Iterations <= 0 {
		options.Iterations = budget.MCTSIterations
	}

	if options.SeedPriorVisits <= 0 {
		options.SeedPriorVisits = budget.MCTSSeedPriorVisits
	}

	if options.MaxReasoningSteps <= 0 {
		options.MaxReasoningSteps = budget.MaxReasoningSteps
	}

	if options.MaxThresholds <= 0 {
		options.MaxThresholds = budget.MaxThresholds
	}

	options.Budget = budget

	return options
}

func workerCount(workers int) int {
	if workers <= 0 {
		return runtime.NumCPU()
	}

	return workers
}
