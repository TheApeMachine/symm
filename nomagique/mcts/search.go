package mcts

import (
	"math"
	"math/rand"
	"slices"

	"github.com/theapemachine/symm/logic/causal"
)

/*
BranchTrace is the observable search statistic of one action branch.
*/
type BranchTrace struct {
	Action     Action
	Visits     int
	MeanReward float64
	RewardStd  float64
	Estimated  ActionEstimate
}

/*
Trace is the search provenance for one decision round.
*/
type Trace struct {
	Iterations          int
	Horizon             int
	ExplorationConstant float64
	UncertaintyWeight   float64
	Branches            []BranchTrace
}

/*
SearchResult is the complete decision result. A bare action without
provenance is insufficient.
*/
type SearchResult struct {
	SelectedAction          Action
	ExpectedEconomicOutcome float64
	OutcomeUncertainty      float64
	Visits                  int
	Alternatives            []ActionEstimate
	CausalModelVersion      string
	SchemaVersion           uint64
	IdentificationStatus    causal.IdentificationStatus
	Trace                   *Trace
	UndefinedActions        []Action
	DecisionUnavailable     bool
}

/*
Search is the MCTS engine over economic states. Selection uses an explicit
UCB-style rule whose exploration constant is declared strategy policy —
expressed in the same units as the economic reward — never disguised as
market mathematics.
*/
type Search struct {
	Iterations          int
	ExplorationConstant float64
	// UncertaintyWeight scales the reward standard error in selection. It is
	// strategy policy (like ExplorationConstant); the standard error carries
	// the causal transition uncertainty sampled by the rollouts.
	UncertaintyWeight float64
	Seed              int64
	rng               *rand.Rand
}

/*
NewSearch builds a deterministic-search engine. Both constants are strategy
policy, expressed in the same units as the economic reward; they are never
derived from market evidence scores.
*/
func NewSearch(iterations int, explorationConstant float64, uncertaintyWeight float64, seed int64) *Search {
	if iterations < 1 {
		iterations = 1
	}

	return &Search{
		Iterations:          iterations,
		ExplorationConstant: explorationConstant,
		UncertaintyWeight:   uncertaintyWeight,
		Seed:                seed,
		rng:                 rand.New(rand.NewSource(seed)),
	}
}

/*
Run searches the action space from the root state under the economic
objective. The selection rule is:

	score(child) = mean(child) + C * sqrt(ln(N_parent) / n_child)

where C is the declared strategy-policy exploration constant in reward units.
If no feasible action has an estimable objective the result is
DecisionUnavailable; it is never reported as a win for Wait.
*/
func (search *Search) Run(rootState State, estimator ActionEstimator) *SearchResult {
	result := &SearchResult{
		IdentificationStatus: causal.IdentificationIdentified,
	}

	if rootState == nil {
		result.DecisionUnavailable = true
		result.IdentificationStatus = causal.IdentificationUndefined
		return result
	}

	possible := rootState.GetPossibleActions()

	if estimator == nil {
		result.UndefinedActions = append([]Action(nil), possible...)
		result.DecisionUnavailable = true
		result.IdentificationStatus = causal.IdentificationNotIdentifiable
		return result
	}

	estimable := make([]Action, 0, len(possible))

	for _, action := range possible {
		estimate := estimator.EstimateAction(rootState, action)
		result.Alternatives = append(result.Alternatives, estimate)

		if estimate.Defined {
			estimable = append(estimable, action)
			continue
		}

		result.UndefinedActions = append(result.UndefinedActions, action)
	}

	if len(estimable) == 0 {
		result.DecisionUnavailable = true
		result.IdentificationStatus = causal.IdentificationNotIdentifiable
		return result
	}

	root := &SearchNode{
		State:          rootState,
		UntakenActions: slices.Clone(estimable),
	}

	for iteration := 0; iteration < search.Iterations; iteration++ {
		selected := search.selectNode(root)

		expanded, err := search.expandNode(selected)

		if err != nil {
			continue
		}

		reward, err := search.rollout(expanded, estimator)

		if err != nil {
			continue
		}

		backpropagate(expanded, reward)
	}

	if len(root.Children) == 0 {
		result.DecisionUnavailable = true
		result.IdentificationStatus = causal.IdentificationNotIdentifiable
		return result
	}

	best := bestChild(root)
	result.SelectedAction = best.Action
	result.ExpectedEconomicOutcome = best.MeanReward()
	result.OutcomeUncertainty = best.StandardError()
	result.Visits = best.Visits
	result.IdentificationStatus = estimator.EstimateAction(rootState, best.Action).IdentificationStatus
	result.Trace = search.trace(root, rootState)

	return result
}

/*
selectNode descends from the root following the UCB rule while every child is
expanded and no untaken action remains.
*/
func (search *Search) selectNode(root *SearchNode) *SearchNode {
	current := root

	for len(current.Children) > 0 && len(current.UntakenActions) == 0 {
		current = ucbChild(current, search.ExplorationConstant, search.UncertaintyWeight)
	}

	return current
}

/*
ucbChild applies the explicit uncertainty-aware UCB selection rule:

	score(child) = mean(child)
	             + C * sqrt(ln(N_parent + 1) / (n_child + 1))
	             + U * std(child) / sqrt(n_child + 1)

C is the policy exploration constant and U the policy uncertainty weight,
both in reward units. The standard error term ties exploration to the
observed reward dispersion, which includes the causal transition noise the
sampled rollouts propagate.
*/
func ucbChild(node *SearchNode, explorationConstant float64, uncertaintyWeight float64) *SearchNode {
	bestScore := math.Inf(-1)
	var best *SearchNode

	for _, child := range node.Children {
		score := child.MeanReward() +
			explorationConstant*math.Sqrt(
				math.Log(float64(node.Visits)+1)/float64(child.Visits+1),
			) +
			uncertaintyWeight*child.RewardStandardDeviation()/math.Sqrt(float64(child.Visits+1))

		if score > bestScore {
			bestScore = score
			best = child
		}
	}

	if best == nil {
		panic("mcts: no selectable child")
	}

	return best
}

/*
expandNode pops one untaken action and applies it, creating a child.
*/
func (search *Search) expandNode(node *SearchNode) (*SearchNode, error) {
	if len(node.UntakenActions) == 0 {
		return node, nil
	}

	action := node.UntakenActions[len(node.UntakenActions)-1]
	node.UntakenActions = node.UntakenActions[:len(node.UntakenActions)-1]

	nextState, err := node.State.ApplyAction(action, search.rng)

	if err != nil {
		return node, err
	}

	child := &SearchNode{
		State:          nextState,
		Action:         action,
		Parent:         node,
		UntakenActions: slices.Clone(nextState.GetPossibleActions()),
		Depth:          node.Depth + 1,
	}
	node.Children = append(node.Children, child)

	return child, nil
}

/*
rollout completes one economic trajectory and returns the accumulated change
in net wealth. The default policy is greedy over the causal action estimates
(argmax expected economic outcome), so rollouts follow economically sensible
continuations rather than uniformly random ones; when no estimate is defined
the action is chosen uniformly at random. The random source samples the
market transition noise, so each rollout walks a plausible causal trajectory.
*/
func (search *Search) rollout(node *SearchNode, estimator ActionEstimator) (float64, error) {
	current := node.State

	for !current.IsTerminal() {
		actions := current.GetPossibleActions()

		if len(actions) == 0 {
			break
		}

		action := search.rolloutAction(current, actions, estimator)
		next, err := current.ApplyAction(action, search.rng)

		if err != nil {
			return 0, err
		}

		current = next
	}

	return current.GetReward(), nil
}

/*
rolloutAction picks the next rollout action: the feasible action with the
highest defined causal expected outcome, or a uniform random action when no
estimate is defined.
*/
func (search *Search) rolloutAction(state State, actions []Action, estimator ActionEstimator) Action {
	if estimator == nil {
		return actions[search.rng.Intn(len(actions))]
	}

	best := actions[0]
	bestValue := math.Inf(-1)
	anyDefined := false

	for _, action := range actions {
		estimate := estimator.EstimateAction(state, action)

		if !estimate.Defined {
			continue
		}

		anyDefined = true

		if estimate.ExpectedOutcome > bestValue {
			bestValue = estimate.ExpectedOutcome
			best = action
		}
	}

	if !anyDefined {
		return actions[search.rng.Intn(len(actions))]
	}

	return best
}

/*
backpropagate aggregates the economic rollout outcome up the tree.
*/
func backpropagate(leaf *SearchNode, reward float64) {
	current := leaf

	for current != nil {
		current.Visits++
		current.TotalReward += reward
		current.SumSquares += reward * reward
		current = current.Parent
	}
}

/*
bestChild is the feasible action with the best search value under the
economic objective: highest mean economic reward, ties broken by visits.
*/
func bestChild(root *SearchNode) *SearchNode {
	var best *SearchNode

	for _, child := range root.Children {
		if child.Visits == 0 {
			continue
		}

		if best == nil || child.MeanReward() > best.MeanReward() ||
			(child.MeanReward() == best.MeanReward() && child.Visits > best.Visits) {
			best = child
		}
	}

	if best == nil {
		panic("mcts: no visited child")
	}

	return best
}

func (search *Search) trace(root *SearchNode, rootState State) *Trace {
	trace := &Trace{
		Iterations:          search.Iterations,
		Horizon:             search.horizon(rootState),
		ExplorationConstant: search.ExplorationConstant,
		UncertaintyWeight:   search.UncertaintyWeight,
		Branches:            make([]BranchTrace, 0, len(root.Children)),
	}

	for _, child := range root.Children {
		trace.Branches = append(trace.Branches, BranchTrace{
			Action:     child.Action,
			Visits:     child.Visits,
			MeanReward: child.MeanReward(),
			RewardStd:  child.RewardStandardDeviation(),
		})
	}

	slices.SortFunc(trace.Branches, func(left BranchTrace, right BranchTrace) int {
		if left.Visits != right.Visits {
			if left.Visits > right.Visits {
				return -1
			}

			return 1
		}

		return 0
	})

	return trace
}

func (search *Search) horizon(rootState State) int {
	if state, supported := rootState.(*EconomicState); supported {
		return state.MaxSteps
	}

	return 0
}
