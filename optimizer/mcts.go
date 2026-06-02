package optimizer

import (
	"context"
	"math"
	"math/rand"
	"runtime"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
MCTSOptions bounds deep TreeSearch after shallow beam seeding.
*/
type MCTSOptions struct {
	Iterations        int
	SeedPriorVisits   int
	MaxReasoningSteps int
	MaxThresholds     int
	Budget            SearchBudget
}

/*
Node is one position in the branch-construction search tree.
*/
type Node struct {
	branches    perspectives.BranchList
	parent      *Node
	children    []*Node
	visits      int
	value       float64
	untried     []Move
	uctDiscount float64
}

/*
TreeSearch runs MCTS over perspective branch registries.
*/
type TreeSearch struct {
	ctx               context.Context
	profile           *Profile
	rows              []perspectives.Measurement
	tape              ReplayTape
	coOccurrence      *CoOccurrenceIndex
	guard             *OverfitGuard
	evaluate          func(perspectives.BranchList) float64
	rng               *rand.Rand
	root              *Node
	best              perspectives.BranchList
	bestScore         float64
	bestClosedTrades  int
	iterations        int
	maxReasoningSteps int
	maxThresholds     int
	cachedMoves       []Move
	onBest            func(BestTree)
	progress          *SearchProgress
	stagnationWindow  int
	explorationWeight float64
	rewardScale       float64
	budget            SearchBudget
}

func normalizeMCTSOptions(
	options MCTSOptions,
	profile *Profile,
	tape ReplayTape,
	workers int,
) MCTSOptions {
	budget := options.Budget

	if budget.IsZero() && profile != nil {
		budget = DeriveSearchBudget(profile, tape, workers)
	}

	return applyBudgetToMCTSOptions(options, budget)
}

/*
NewHybridTreeSearch seeds MCTS root nodes from shallow beam survivors.
*/
func NewHybridTreeSearch(
	ctx context.Context,
	profile *Profile,
	rows []perspectives.Measurement,
	guardOptions GuardOptions,
	seeds []CandidateScore,
	options MCTSOptions,
) *TreeSearch {
	return NewHybridTreeSearchWithTape(
		ctx, profile, rows, ReplayTape{}, guardOptions, seeds, options,
	)
}

/*
NewHybridTreeSearchWithTape reuses a precompiled replay tape across MCTS rollouts.
*/
func NewHybridTreeSearchWithTape(
	ctx context.Context,
	profile *Profile,
	rows []perspectives.Measurement,
	tape ReplayTape,
	guardOptions GuardOptions,
	seeds []CandidateScore,
	options MCTSOptions,
) *TreeSearch {
	workers := runtime.NumCPU()
	options = normalizeMCTSOptions(options, profile, tape, workers)
	budget := options.Budget

	if budget.IsZero() {
		budget = DeriveSearchBudget(profile, tape, workers)
	}

	profile.PrepareCache()

	if tape.Len() == 0 {
		tape = PrecompileTape(rows)
	}

	guard := NewOverfitGuard(ctx, guardOptions, tape, profile)

	search := &TreeSearch{
		ctx:               ctx,
		profile:           profile,
		rows:              rows,
		tape:              tape,
		coOccurrence:      NewCoOccurrenceIndex(tape, budget.MinChainSupport),
		guard:             guard,
		rng:               rand.New(rand.NewSource(rand.Int63())),
		iterations:        options.Iterations,
		maxReasoningSteps: options.MaxReasoningSteps,
		maxThresholds:     options.MaxThresholds,
		explorationWeight: budget.ExplorationWeight,
		rewardScale:       budget.MCTSRewardScale,
		budget:            budget,
	}

	search.evaluate = search.scoreBranches
	search.cachedMoves = search.generateAllMoves()
	search.root = &Node{branches: perspectives.BranchList{}}
	search.bestScore = math.Inf(-1)
	search.bestClosedTrades = -1
	search.best = perspectives.BranchList{}
	search.progress = NewSearchProgress()
	search.stagnationWindow = budget.BeamWidth
	search.seedRoot(seeds, options.SeedPriorVisits)

	return search
}

func (search *TreeSearch) seedRoot(seeds []CandidateScore, priorVisits int) {
	for _, seed := range seeds {
		score := search.evaluate(seed.Branches)

		if perspectives.HasInvalidTopLevelDenySiblings(seed.Branches) {
			continue
		}

		canonical := perspectives.CanonicalPlaybookBranches(seed.Branches)
		reward := search.normalizeMCTSReward(score)

		child := &Node{
			branches: canonical.Clone(),
			parent:   search.root,
			visits:   priorVisits,
			value:    reward * float64(priorVisits),
			untried:  search.moves(canonical),
		}
		search.root.children = append(search.root.children, child)
		search.root.visits += priorVisits

		replay := NewReplaySimulationWithTape(
			search.ctx, canonical, search.tape,
		).Result()

		if search.guard.ImprovesPersistedBest(
			score, replay.ClosedTrades, search.bestScore, search.bestClosedTrades,
		) && search.guard.PersistCandidate(canonical) {
			search.bestScore = score
			search.bestClosedTrades = replay.ClosedTrades
			search.best = canonical.Clone()
			search.emitBest(0, canonical, score)
		}
	}

	if len(search.root.children) == 0 {
		search.root.untried = search.moves(search.root.branches)
	}
}

/*
Run finds the most profitable branch registry for the replay profile.
*/
func (search *TreeSearch) Run() perspectives.BranchList {
	for iteration := 0; iteration < search.iterations; iteration++ {
		if search.progress != nil &&
			search.progress.Stagnant(search.stagnationWindow) &&
			iteration > 0 {
			bestScore := search.progress.BestScore()

			if math.IsInf(bestScore, 0) {
				TuneLog(
					"mcts pivot at iteration %d/%d: reward stalled, nesting entry gates",
					iteration+1,
					search.iterations,
				)
			} else {
				TuneLog(
					"mcts pivot at iteration %d/%d: reward stalled at %.6f, nesting entry gates",
					iteration+1,
					search.iterations,
					bestScore,
				)
			}

			search.progress.ResetStagnation()
		}

		node := search.selectNode()
		child := search.expand(node)
		reward := search.rollout(iteration, child)
		search.backpropagate(child, reward)

		if iteration == 0 ||
			(iteration+1)%32 == 0 ||
			iteration+1 == search.iterations {
			bestScore := search.bestScore

			if math.IsInf(bestScore, 0) {
				TuneLog("mcts iteration %d/%d (no persistable best yet)", iteration+1, search.iterations)
			} else {
				TuneLog("mcts iteration %d/%d best realized score %.6f", iteration+1, search.iterations, bestScore)
			}
		}
	}

	return search.best.Clone()
}

func (search *TreeSearch) walkForwardFinalists(
	seeds []CandidateScore,
) []CandidateScore {
	pool := make([]CandidateScore, 0, len(seeds)+1)

	if len(search.best) > 0 {
		pool = append(pool, CandidateScore{
			Score:         search.bestScore,
			AdjustedScore: search.bestScore,
			ClosedTrades:  search.bestClosedTrades,
			Branches:      search.best.Clone(),
		})
	}

	pool = append(pool, seeds...)

	return dedupeCandidatesByBranch(pool)
}
