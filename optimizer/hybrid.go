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

	shallowOptions := options.ScanOptions
	shallowOptions.Guard = options.Guard

	if options.ShallowDepth > 0 {
		shallowOptions.MaxReasoningSteps = options.ShallowDepth
	}

	search := NewScanSearch(ctx, profile, rows, shallowOptions)
	search.onBest = options.OnBest
	search.onCandidate = options.OnCandidate

	seeds, scanStats := search.RunTopK(options.SeedCount)
	seeds = mergeDepthSeeds(seeds, search.beamScoresClone(), options.SeedCount)

	mcts := NewHybridTreeSearch(ctx, profile, rows, options.Guard, seeds, options.MCTSOptions)
	mcts.onBest = options.OnBest

	branches := mcts.Run()

	if options.Guard.WalkForward.Enabled && len(branches) > 0 {
		guard := NewOverfitGuard(ctx, options.Guard, PrecompileTape(rows))
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
	if limit <= 0 {
		return top
	}

	if entry.AdjustedScore <= 0 &&
		perspectives.HasInvalidTopLevelDenySiblings(entry.Branches) {
		return top
	}

	return insertDepthStratifiedBeam(top, entry, limit)
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

		if left.Score != right.Score {
			return left.Score > right.Score
		}

		return reasoningDepth(left.Branches) > reasoningDepth(right.Branches)
	})

	if len(top) > limit {
		top = top[:limit]
	}

	return top
}

func mergeDepthSeeds(
	scored []CandidateScore, beam []CandidateScore, limit int,
) []CandidateScore {
	if limit <= 0 {
		return scored
	}

	merged := append([]CandidateScore(nil), scored...)
	seen := make(map[string]struct{}, len(merged))

	for _, entry := range merged {
		seen[branchListKey(entry.Branches)] = struct{}{}
	}

	deepest := append([]CandidateScore(nil), beam...)
	sort.Slice(deepest, func(leftIndex, rightIndex int) bool {
		leftDepth := reasoningDepth(deepest[leftIndex].Branches)
		rightDepth := reasoningDepth(deepest[rightIndex].Branches)

		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}

		return deepest[leftIndex].AdjustedScore > deepest[rightIndex].AdjustedScore
	})

	for _, entry := range deepest {
		if len(merged) >= limit {
			break
		}

		key := branchListKey(entry.Branches)

		if _, ok := seen[key]; ok {
			continue
		}

		if entry.AdjustedScore <= 0 &&
			perspectives.HasInvalidTopLevelDenySiblings(entry.Branches) {
			continue
		}

		merged = append(merged, entry)
		seen[key] = struct{}{}
	}

	return merged
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
