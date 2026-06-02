package optimizer

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
	ScanOptions ScanOptions
	MCTSOptions MCTSOptions
	SeedCount   int
	Guard       GuardOptions
	OnBest      func(BestTree)
	OnCandidate func(CandidateScore)
}

/*
RunHybridSearch scores multi-signal beam candidates, then deepens seeds with MCTS.
*/
func RunHybridSearch(
	ctx context.Context,
	profile *Profile,
	rows []perspectives.Measurement,
	tape ReplayTape,
	options HybridOptions,
) (perspectives.BranchList, HybridStats, error) {
	if options.SeedCount <= 0 {
		budget := options.ScanOptions.Budget

		if budget.IsZero() {
			budget = DeriveSearchBudget(profile, tape, options.ScanOptions.Workers)
		}

		options.SeedCount = budget.HybridSeedCount
	}

	search := NewScanSearchWithTape(ctx, profile, rows, tape, options.ScanOptions)
	search.onBest = options.OnBest
	search.onCandidate = options.OnCandidate

	TuneLog("beam search (max depth %d)", options.ScanOptions.MaxReasoningSteps)

	seeds, scanStats := search.RunTopK(options.SeedCount)
	seeds = selectHybridSeeds(seeds, search.beamScoresClone(), options.SeedCount)

	TuneLog("mcts search (%d seeds, %d iterations)", len(seeds), options.MCTSOptions.Iterations)

	mcts := NewHybridTreeSearchWithTape(
		ctx, profile, rows, tape, options.Guard, seeds, options.MCTSOptions,
	)
	mcts.SetStagnationWindow(options.ScanOptions.BeamWidth)
	mcts.onBest = options.OnBest

	branches := mcts.Run()

	if options.Guard.WalkForward.Enabled {
		TuneLog("walk-forward validation")

		guard := NewOverfitGuard(ctx, options.Guard, tape, profile)
		branches = SelectWalkForwardBest(
			guard,
			rows,
			mcts.walkForwardFinalists(seeds),
		)
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
	if limit <= 0 || !trainSeedEligible(entry) {
		return top
	}

	return insertScoreBeam(top, entry, limit)
}

/*
selectHybridSeeds merges guard-valid top-K results with the full depth-stratified
beam so MCTS receives both profitable and structurally deep starting points.
*/
func selectHybridSeeds(
	scored []CandidateScore, beam []CandidateScore, limit int,
) []CandidateScore {
	if limit <= 0 {
		return scored
	}

	pool := append(append([]CandidateScore(nil), scored...), beam...)

	return collapseScoreBeam(pool, limit)
}

func insertBeam(
	top []CandidateScore, entry CandidateScore, limit int,
) []CandidateScore {
	if limit <= 0 {
		return top
	}

	top = append(top, entry)
	sort.Slice(top, func(leftIndex, rightIndex int) bool {
		left := top[leftIndex]
		right := top[rightIndex]

		return compareBeamCandidates(left, right)
	})

	if len(top) > limit {
		top = top[:limit]
	}

	return top
}

func mergeDepthSeeds(
	scored []CandidateScore, beam []CandidateScore, limit int,
) []CandidateScore {
	return selectHybridSeeds(scored, beam, limit)
}

func branchListKey(branches perspectives.BranchList) string {
	canonical := perspectives.CanonicalPlaybookBranches(branches)
	parts := make([]string, 0, len(canonical))

	for _, branch := range canonical {
		parts = append(parts, branchFingerprint(branch))
	}

	return strings.Join(parts, "|")
}

func branchFingerprint(branch perspectives.Branch) string {
	children := make([]string, 0, len(branch.Branches))

	for _, child := range branch.Branches {
		children = append(children, branchFingerprint(child))
	}

	return fmt.Sprintf(
		"%s:%d:%d:%d:%.12g:%d[%s]",
		branch.Category,
		branch.Observation,
		branch.Unit,
		branch.Condition,
		branch.Value,
		branch.Action.Type,
		strings.Join(children, ","),
	)
}
