package mcts

import (
	"context"
	"math"
	"math/rand"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/beam"
	"github.com/theapemachine/symm/optimizer/budget"
	"github.com/theapemachine/symm/optimizer/cooccurrence"
	"github.com/theapemachine/symm/optimizer/guard"
	"github.com/theapemachine/symm/optimizer/log"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/progress"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
TreeSearch runs MCTS over perspective branch registries.
*/
type TreeSearch struct {
	ctx               context.Context
	pool              *qpool.Q
	profile           *profile.Profile
	rows              []perspectives.Measurement
	tape              replay.ReplayTape
	coOccurrence      *cooccurrence.CoOccurrenceIndex
	guard             *guard.OverfitGuard
	evaluate          func(perspectives.BranchList) float64
	rng               *rand.Rand
	best              perspectives.BranchList
	bestScore         float64
	bestClosedTrades  int
	iterations        int
	maxReasoningSteps int
	maxThresholds     int
	OnBest            func(types.BestTree)
	progress          *progress.SearchProgress
	stagnationWindow  int
	rewardScale       float64
	budget            types.SearchBudget
	seedCandidates    []types.CandidateScore
	seedPriorVisits   int
	seeded            bool

	moves     *Moves
	heuristic *Heuristic
	tree      *Tree
	rollout   *Rollout
	seeds     *Seeds
}

func NewHybridTreeSearch(
	ctx context.Context,
	profile *profile.Profile,
	rows []perspectives.Measurement,
	guardOptions types.GuardOptions,
	seeds []types.CandidateScore,
	options Options,
	pool *qpool.Q,
) *TreeSearch {
	return NewHybridTreeSearchWithTape(
		ctx, profile, rows, replay.ReplayTape{}, guardOptions, seeds, options, pool,
	)
}

func NewHybridTreeSearchWithTape(
	ctx context.Context,
	profile *profile.Profile,
	rows []perspectives.Measurement,
	tape replay.ReplayTape,
	guardOptions types.GuardOptions,
	seeds []types.CandidateScore,
	options Options,
	pool *qpool.Q,
) *TreeSearch {
	workers := workerCount(0)
	options = normalizeOptions(options, profile, tape, workers)
	searchBudget := options.Budget

	if searchBudget.IsZero() {
		searchBudget = budget.DeriveSearchBudget(profile, tape, workers)
		options.Budget = searchBudget
	}

	profile.PrepareCache()

	if tape.Len() == 0 {
		tape = replay.PrecompileTape(rows)
	}

	overfitGuard := guard.NewOverfitGuard(ctx, guardOptions, tape, profile)
	coOccurrenceIndex := cooccurrence.NewCoOccurrenceIndex(tape, searchBudget.MinChainSupport)
	moves := NewMoves(profile, coOccurrenceIndex, options.MaxThresholds, options.MaxReasoningSteps)
	searchProgress := progress.NewSearchProgress()

	search := &TreeSearch{
		ctx:               ctx,
		pool:              pool,
		profile:           profile,
		rows:              rows,
		tape:              tape,
		coOccurrence:      coOccurrenceIndex,
		guard:             overfitGuard,
		rng:               rand.New(rand.NewSource(rand.Int63())),
		iterations:        options.Iterations,
		maxReasoningSteps: options.MaxReasoningSteps,
		maxThresholds:     options.MaxThresholds,
		rewardScale:       searchBudget.MCTSRewardScale,
		budget:            searchBudget,
		bestScore:         math.Inf(-1),
		bestClosedTrades:  -1,
		best:              perspectives.BranchList{},
		progress:          searchProgress,
		stagnationWindow:  searchBudget.BeamWidth,
		seedCandidates:    seeds,
		seedPriorVisits:   options.SeedPriorVisits,
		moves:             moves,
	}

	search.evaluate = search.scoreBranches
	search.heuristic = NewHeuristic(
		moves, profile, searchProgress, search.stagnationWindow, search.rng,
	)
	search.tree = NewTree(searchBudget.ExplorationWeight, moves, search.heuristic)
	search.rollout = NewRollout(search)
	search.seeds = NewSeeds(search)

	return search
}

func (search *TreeSearch) Run() perspectives.BranchList {
	if !search.seeded {
		search.seeded = true
		search.seeds.Run(search.seedCandidates, search.seedPriorVisits)
	}

	reporter := progress.NewTuneProgressReporter(search.iterations)
	log.TuneLog("mcts rollouts starting (%d iterations)", search.iterations)

	for iteration := 0; iteration < search.iterations; iteration++ {
		if search.progress != nil &&
			search.progress.Stagnant(search.stagnationWindow) &&
			iteration > 0 {
			bestScore := search.progress.BestScore()

			if isInf(bestScore) {
				log.TuneLog(
					"mcts pivot at iteration %d/%d: reward stalled, nesting entry gates",
					iteration+1,
					search.iterations,
				)
			} else {
				log.TuneLog(
					"mcts pivot at iteration %d/%d: reward stalled at %.6f, nesting entry gates",
					iteration+1,
					search.iterations,
					bestScore,
				)
			}

			search.progress.ResetStagnation()
		}

		node := search.tree.Select()
		child := search.tree.Expand(node)
		reward := search.rollout.Run(iteration, child)
		search.tree.Backpropagate(child, reward)

		if reporter.ShouldLog(iteration + 1) {
			reporter.MarkLogged()
			logRolloutProgress(search, iteration+1, reporter.Elapsed())
		}
	}

	log.TuneLog(
		"mcts rollouts finished (%d iterations, %s)",
		search.iterations,
		reporter.Elapsed(),
	)

	return search.best.Clone()
}

func (search *TreeSearch) SetStagnationWindow(beamWidth int) {
	if beamWidth > 0 {
		search.stagnationWindow = beamWidth
		search.heuristic.stagnationWindow = beamWidth
	}
}

func (search *TreeSearch) Iterations() int {
	return search.iterations
}

func (search *TreeSearch) Tree() *Tree {
	return search.tree
}

func (search *TreeSearch) Moves() *Moves {
	return search.moves
}

func (search *TreeSearch) WalkForwardFinalists(
	seeds []types.CandidateScore,
) []types.CandidateScore {
	pool := make([]types.CandidateScore, 0, len(seeds)+1)

	if len(search.best) > 0 {
		pool = append(pool, types.CandidateScore{
			Score:         search.bestScore,
			AdjustedScore: search.bestScore,
			ClosedTrades:  search.bestClosedTrades,
			Branches:      search.best.Clone(),
		})
	}

	pool = append(pool, seeds...)

	return beam.DedupeCandidatesByBranch(pool)
}

func (search *TreeSearch) scoreBranches(branches perspectives.BranchList) float64 {
	canonical := perspectives.CanonicalPlaybookBranches(branches)
	raw := replay.NewReplaySimulationWithTape(
		search.ctx, canonical, search.tape,
	).Result().Score

	return search.guard.AdjustedScore(raw, canonical)
}

func (search *TreeSearch) emitBest(
	iteration int, branches perspectives.BranchList, score float64,
) {
	if search.OnBest == nil {
		return
	}

	search.OnBest(types.BestTree{
		Iteration: iteration + 1,
		Score:     score,
		Branches:  branches.Clone(),
	})
}

func (search *TreeSearch) SeedRoot(candidates []types.CandidateScore, priorVisits int) {
	search.seeds.Run(candidates, priorVisits)
}
