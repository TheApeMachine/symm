package optimizer

import (
	"context"
	"math"
	"math/rand"

	"github.com/theapemachine/symm/market/perspectives"
)

const (
	DefaultMCTSIterations      = 1024
	DefaultHybridSeedCount     = 50
	DefaultHybridShallowDepth  = 4
	DefaultMCTSSeedPriorVisits = 10
	DefaultMCTSMaxThresholds   = 0
	explorationWeight          = 1.41
	mctsRewardSigmoidScale     = 10.0
)

/*
MCTSOptions bounds deep TreeSearch after shallow beam seeding.
*/
type MCTSOptions struct {
	Iterations        int
	SeedPriorVisits   int
	MaxReasoningSteps int
	MaxThresholds     int
}

/*
Node is one position in the branch-construction search tree.
*/
type Node struct {
	branches perspectives.BranchList
	parent   *Node
	children []*Node
	visits   int
	value    float64
	untried  []Move
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
}

func normalizeMCTSOptions(options MCTSOptions) MCTSOptions {
	if options.Iterations <= 0 {
		options.Iterations = DefaultMCTSIterations
	}

	if options.SeedPriorVisits <= 0 {
		options.SeedPriorVisits = DefaultMCTSSeedPriorVisits
	}

	if options.MaxReasoningSteps <= 0 {
		options.MaxReasoningSteps = DefaultMaxReasoningSteps
	}

	if options.MaxThresholds < 0 {
		options.MaxThresholds = DefaultMCTSMaxThresholds
	}

	return options
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
	options = normalizeMCTSOptions(options)
	profile.PrepareCache()
	tape := PrecompileTape(rows)
	guard := NewOverfitGuard(ctx, guardOptions, tape)

	search := &TreeSearch{
		ctx:               ctx,
		profile:           profile,
		rows:              rows,
		tape:              tape,
		coOccurrence:      NewCoOccurrenceIndex(tape),
		guard:             guard,
		rng:               rand.New(rand.NewSource(rand.Int63())),
		iterations:        options.Iterations,
		maxReasoningSteps: options.MaxReasoningSteps,
		maxThresholds:     options.MaxThresholds,
	}

	search.evaluate = search.scoreBranches
	search.cachedMoves = search.generateAllMoves()
	search.root = &Node{branches: perspectives.BranchList{}}
	search.bestScore = math.Inf(-1)
	search.bestClosedTrades = -1
	search.best = perspectives.BranchList{}
	search.seedRoot(seeds, options.SeedPriorVisits)

	return search
}

func (search *TreeSearch) scoreBranches(branches perspectives.BranchList) float64 {
	canonical := perspectives.CanonicalPlaybookBranches(branches)
	raw := NewReplaySimulationWithTape(
		search.ctx, canonical, search.tape,
	).Result().Score

	return search.guard.AdjustedScore(raw, canonical)
}

func (search *TreeSearch) seedRoot(seeds []CandidateScore, priorVisits int) {
	for _, seed := range seeds {
		score := search.evaluate(seed.Branches)

		if perspectives.HasInvalidTopLevelDenySiblings(seed.Branches) {
			continue
		}

		canonical := perspectives.CanonicalPlaybookBranches(seed.Branches)
		reward := normalizeMCTSReward(score)

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
		node := search.selectNode()
		child := search.expand(node)
		reward := search.rollout(iteration, child)
		search.backpropagate(child, reward)
	}

	return search.best.Clone()
}

func (search *TreeSearch) selectNode() *Node {
	node := search.root

	for len(node.children) > 0 && len(node.untried) == 0 {
		node = search.bestChild(node)
	}

	return node
}

func (search *TreeSearch) bestChild(node *Node) *Node {
	best := node.children[0]
	bestScore := search.uct(node, best)

	for _, child := range node.children[1:] {
		score := search.uct(node, child)

		if score > bestScore {
			best = child
			bestScore = score
		}
	}

	return best
}

func (search *TreeSearch) uct(parent, child *Node) float64 {
	if child.visits == 0 {
		return math.Inf(1)
	}

	exploit := child.value / float64(child.visits)
	explore := explorationWeight * math.Sqrt(
		math.Log(float64(parent.visits))/float64(child.visits),
	)

	return exploit + explore
}

func (search *TreeSearch) expand(node *Node) *Node {
	if len(node.untried) == 0 {
		return node
	}

	moveIndex := search.sampleMoveIndex(node.untried, node.branches)
	move := node.untried[moveIndex]
	node.untried[moveIndex] = node.untried[len(node.untried)-1]
	node.untried = node.untried[:len(node.untried)-1]

	childBranches := search.applyMove(node.branches, move)

	child := &Node{
		branches: childBranches,
		parent:   node,
		untried:  search.moves(childBranches),
	}

	node.children = append(node.children, child)

	return child
}

func (search *TreeSearch) rollout(iteration int, node *Node) float64 {
	branches := node.branches.Clone()

	for step := 0; step < search.maxReasoningSteps; step++ {
		moves := search.reachableMoves(search.allMoves(), branches)

		if len(moves) == 0 {
			break
		}

		move := search.sampleRolloutMove(moves, branches)
		branches = search.applyMove(branches, move)
	}

	score := search.evaluate(branches)

	if len(branches) > 0 {
		canonical := perspectives.CanonicalPlaybookBranches(branches)
		replay := NewReplaySimulationWithTape(
			search.ctx, canonical, search.tape,
		).Result()

		if search.guard.ImprovesPersistedBest(
			score, replay.ClosedTrades, search.bestScore, search.bestClosedTrades,
		) && search.guard.PersistCandidate(canonical) {
			search.bestScore = score
			search.bestClosedTrades = replay.ClosedTrades
			search.best = canonical.Clone()
			search.emitBest(iteration, canonical, score)
		}
	}

	return normalizeMCTSReward(score)
}

func (search *TreeSearch) emitBest(
	iteration int, branches perspectives.BranchList, score float64,
) {
	if search.onBest == nil {
		return
	}

	search.onBest(BestTree{
		Iteration: iteration + 1,
		Score:     score,
		Branches:  branches.Clone(),
	})
}

func (search *TreeSearch) backpropagate(node *Node, reward float64) {
	for current := node; current != nil; current = current.parent {
		current.visits++
		current.value += reward
	}
}

func (search *TreeSearch) moves(
	branches perspectives.BranchList,
) []Move {
	if reasoningDepth(branches) >= search.maxReasoningSteps {
		return nil
	}

	return search.reachableMoves(search.allMoves(), branches)
}

func normalizeMCTSReward(pnlScore float64) float64 {
	return 1.0 / (1.0 + math.Exp(-pnlScore*mctsRewardSigmoidScale))
}

func (search *TreeSearch) allMoves() []Move {
	return search.cachedMoves
}

func (search *TreeSearch) generateAllMoves() []Move {
	categories := search.profile.Categories()
	moves := make([]Move, 0)

	for _, category := range categories {
		for _, observation := range searchObservations {
			actions := search.actions(observation)

			for _, regime := range searchRegimes {
				for _, unit := range searchUnits {
					values := search.profile.AdaptiveValues(
						category,
						unit,
						search.maxThresholds,
					)

					for _, condition := range searchConditions {
						for _, value := range values {
							for _, action := range actions {
								moves = append(moves, Move{
									category:    category,
									observation: observation,
									regime:      regime,
									condition:   condition,
									unit:        unit,
									value:       value,
									action:      action,
								})
							}
						}
					}
				}
			}
		}
	}

	return moves
}

func (search *TreeSearch) actions(
	observation perspectives.ObservationType,
) []perspectives.ActionType {
	switch observation {
	case perspectives.ObservationNotHolding:
		return searchEntryActions
	case perspectives.ObservationHolding:
		return searchExitActions
	default:
		return []perspectives.ActionType{perspectives.ActionNone}
	}
}

func (search *TreeSearch) applyMove(
	branches perspectives.BranchList, move Move,
) perspectives.BranchList {
	branch := search.branchFromMove(move)

	if len(branches) == 0 {
		return perspectives.BranchList{branch}
	}

	switch move.observation {
	case perspectives.ObservationNone:
		if perspectives.FindEntryIndex(branches) >= 0 {
			nested, ok := nestGateUnderEntry(branches, branch)

			if ok {
				return nested
			}
		}

		return branches.Clone()
	case perspectives.ObservationNotHolding:
		return appendEntryPathSibling(branches, branch)
	case perspectives.ObservationHolding:
		return appendExitSibling(branches, branch)
	default:
		return append(branches.Clone(), branch)
	}
}
