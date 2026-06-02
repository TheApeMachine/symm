package optimizer

import (
	"context"
	"sort"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
HybridStats reports shallow beam and deep MCTS search activity.
*/
type HybridStats struct {
	Scan       ScanStats
	SeedCount  int
	MCTSRounds int
}

/*
HybridOptions configures progressive deepening: beam trunk, MCTS branches.
*/
type HybridOptions struct {
	ScanOptions  ScanOptions
	MCTSOptions  MCTSOptions
	SeedCount    int
	ShallowDepth int
	Guard        GuardOptions
	OnBest       func(BestTree)
	OnCandidate  func(CandidateScore)
}

/*
RunHybridSearch exhaustively scans shallow trees, then deepens seeds with MCTS.
*/
func RunHybridSearch(
	ctx context.Context,
	profile *Profile,
	rows []perspectives.Measurement,
	options HybridOptions,
) (perspectives.BranchList, HybridStats, error) {
	if options.SeedCount <= 0 {
		options.SeedCount = DefaultHybridSeedCount
	}

	if options.ShallowDepth <= 0 {
		options.ShallowDepth = DefaultHybridShallowDepth
	}

	shallowOptions := options.ScanOptions
	shallowOptions.MaxReasoningSteps = options.ShallowDepth
	shallowOptions.Guard = options.Guard

	search := NewScanSearch(ctx, profile, rows, shallowOptions)
	search.onBest = options.OnBest
	search.onCandidate = options.OnCandidate

	seeds, scanStats := search.RunTopK(options.SeedCount)

	mcts := NewHybridTreeSearch(ctx, profile, rows, options.Guard, seeds, options.MCTSOptions)
	mcts.onBest = options.OnBest

	branches := mcts.Run()

	if options.Guard.WalkForward.Enabled && len(branches) > 0 {
		guard := NewOverfitGuard(ctx, options.Guard)
		ok, _ := guard.ValidateWalkForward(branches, rows)

		if !ok {
			branches = perspectives.BranchList{}
		}
	}

	stats := HybridStats{
		Scan:       scanStats,
		SeedCount:  len(seeds),
		MCTSRounds: mcts.iterations,
	}

	return branches, stats, nil
}

func insertTopK(
	top []CandidateScore, entry CandidateScore, limit int,
) []CandidateScore {
	if limit <= 0 || entry.Score <= 0 {
		return top
	}

	top = append(top, entry)
	sort.Slice(top, func(leftIndex, rightIndex int) bool {
		return top[leftIndex].Score > top[rightIndex].Score
	})

	if len(top) > limit {
		top = top[:limit]
	}

	return top
}
