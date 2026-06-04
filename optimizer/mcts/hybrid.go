package mcts

import (
	"context"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/beam"
	"github.com/theapemachine/symm/optimizer/budget"
	"github.com/theapemachine/symm/optimizer/combine"
	"github.com/theapemachine/symm/optimizer/guard"
	"github.com/theapemachine/symm/optimizer/log"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/scan"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
HybridStats reports shallow beam and deep MCTS search activity.
*/
type HybridStats struct {
	Scan       types.ScanStats
	SeedCount  int
	MCTSRounds int
}

/*
HybridOptions configures progressive deepening: beam trunk, MCTS branches.
*/
type HybridOptions struct {
	ScanOptions types.ScanOptions
	MCTSOptions Options
	SeedCount   int
	Guard       types.GuardOptions
	OnBest      func(types.BestTree)
	OnCandidate func(types.CandidateScore)
}

/*
RunHybridSearch scores multi-signal beam candidates, then deepens seeds with MCTS.
*/
func RunHybridSearch(
	ctx context.Context,
	profile *profile.Profile,
	rows []perspectives.Measurement,
	tape replay.ReplayTape,
	options HybridOptions,
) (perspectives.BranchList, HybridStats, error) {
	if options.SeedCount <= 0 {
		searchBudget := options.ScanOptions.Budget

		if searchBudget.IsZero() {
			searchBudget = budget.DeriveSearchBudget(profile, tape, options.ScanOptions.Workers)
		}

		options.SeedCount = searchBudget.HybridSeedCount
	}

	search := scan.NewScanSearchWithTape(ctx, profile, rows, tape, options.ScanOptions)
	search.OnBest = options.OnBest
	search.OnCandidate = options.OnCandidate

	log.TuneLog("beam search (max depth %d)", options.ScanOptions.MaxReasoningSteps)

	seeds, scanStats := search.RunTopK(options.SeedCount)
	seeds = beam.SelectHybridSeeds(seeds, search.BeamScoresClone(), options.SeedCount)

	log.TuneLog("mcts search (%d seeds, %d iterations)", len(seeds), options.MCTSOptions.Iterations)

	mctsSearch := NewHybridTreeSearchWithTape(
		ctx, profile, rows, tape, options.Guard, seeds, options.MCTSOptions,
		options.ScanOptions.Pool,
	)
	mctsSearch.SetStagnationWindow(options.ScanOptions.BeamWidth)
	mctsSearch.OnBest = options.OnBest

	branches := mctsSearch.Run()

	branches = combineStrategies(mctsSearch, seeds, branches)

	if options.Guard.WalkForward.Enabled {
		log.TuneLog("walk-forward validation")

		overfitGuard := guard.NewOverfitGuard(ctx, options.Guard, tape, profile)
		branches = guard.SelectWalkForwardBest(
			overfitGuard,
			rows,
			mctsSearch.WalkForwardFinalists(seeds),
			options.ScanOptions.Pool,
		)
	}

	stats := HybridStats{
		Scan:       scanStats,
		SeedCount:  len(seeds),
		MCTSRounds: mctsSearch.Iterations(),
	}

	return branches, stats, nil
}

/*
combineStrategies runs the second search round: it composes the distinct
strategies discovered across the finalists into one multi-entry playbook and
adopts it only when its complexity-adjusted joint score beats the single best
tree. scoreBranches applies the overfit complexity penalty, so a combined tree
that merely adds gates without earning joint return is rejected.
*/
func combineStrategies(
	search *TreeSearch,
	seeds []types.CandidateScore,
	best perspectives.BranchList,
) perspectives.BranchList {
	finalists := search.WalkForwardFinalists(seeds)

	if len(finalists) < 2 {
		return best
	}

	pool := make([]combine.Scored, 0, len(finalists))

	for _, candidate := range finalists {
		pool = append(pool, combine.Scored{
			Branches: candidate.Branches,
			Score:    candidate.AdjustedScore,
		})
	}

	combined := combine.Greedy(pool, search.scoreBranches)

	if len(combined) == 0 {
		return best
	}

	if len(perspectives.FindAllEntryIndices(combined)) < 2 {
		return best
	}

	if search.scoreBranches(combined) <= search.scoreBranches(best) {
		return best
	}

	log.TuneLog(
		"combination round: %d strategies merged",
		len(perspectives.FindAllEntryIndices(combined)),
	)

	return combined
}
