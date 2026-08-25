package mcts

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
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
	IdentificationStatus    IdentificationStatus
	Trace                   *Trace
	UndefinedActions        []Action
	DecisionUnavailable     bool
	// Tree is the explored search tree rooted at the search origin. It is
	// retained so the decision trace can project the actual per-node economic
	// statistics the search aggregated, rather than a flat branch list.
	Tree *SearchNode
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
	// dimensionless strategy policy: the standard error it scales already
	// carries reward units, so the weighted term stays in reward units like
	// the mean and the C exploration term.
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
		IdentificationStatus: IdentificationIdentified,
	}

	if rootState == nil {
		result.DecisionUnavailable = true
		result.IdentificationStatus = IdentificationUndefined
		return result
	}

	possible := rootState.GetPossibleActions()

	if estimator == nil {
		result.UndefinedActions = append([]Action(nil), possible...)
		result.DecisionUnavailable = true
		result.IdentificationStatus = IdentificationNotIdentifiable
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
		result.IdentificationStatus = IdentificationNotIdentifiable
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

		reward, err := search.simulate(expanded, estimator)

		if err != nil {
			continue
		}

		backpropagate(expanded, reward)
	}

	if len(root.Children) == 0 {
		result.DecisionUnavailable = true
		result.IdentificationStatus = IdentificationNotIdentifiable
		return result
	}

	best, found := bestChild(root)

	if !found {
		// Every expanded child has zero visits (all rollouts failed); the
		// economic objective could not be evaluated. This is an explicit
		// unavailable result, not a Wait win.
		result.DecisionUnavailable = true
		result.IdentificationStatus = IdentificationInsufficientSupport
		return result
	}

	result.SelectedAction = best.Action
	result.ExpectedEconomicOutcome = best.MeanReward()
	result.OutcomeUncertainty = best.StandardError()
	result.Visits = best.Visits
	result.IdentificationStatus = alternativeEstimate(result.Alternatives, best.Action).IdentificationStatus
	result.Trace = search.trace(root, rootState, result.Alternatives)
	result.Tree = root

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

C is the policy exploration constant in reward units; U is the policy
uncertainty weight and is dimensionless (the standard error it scales
already carries reward units). The standard error term ties exploration to
the observed reward dispersion, which includes the causal transition noise
the sampled rollouts propagate.
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
simulate evaluates one node by re-deriving its state from the root along the
tree path, applying each action with a fresh causal innovation sample, then
completing a greedy rollout. Re-deriving per visit is what makes the search
integrate over the stochastic transition distribution: a root action is not
frozen to the single first-step realization sampled at expansion time, and
each visit to the same action draws a new market realization. The random
source samples the market transition noise, so each trajectory walks a
plausible causal path.
*/
func (search *Search) simulate(node *SearchNode, estimator ActionEstimator) (float64, error) {
	state, err := search.derivePath(node)

	if err != nil {
		return 0, err
	}

	for !state.IsTerminal() {
		actions := state.GetPossibleActions()

		if len(actions) == 0 {
			break
		}

		action := search.rolloutAction(state, actions, estimator)
		next, err := state.ApplyAction(action, search.rng)

		if err != nil {
			return 0, err
		}

		state = next
	}

	return state.GetReward(), nil
}

/*
derivePath reconstructs the state at a tree node from the root, applying each
action along the path with a fresh sample of the market transition. The
stored intermediate states are expansion templates only; the stochastic
first transitions are resampled on every visit so branch statistics
aggregate the true transition distribution.
*/
func (search *Search) derivePath(node *SearchNode) (State, error) {
	if node == nil {
		return nil, fmt.Errorf("mcts: cannot derive a nil node")
	}

	if node.Parent == nil {
		return node.State, nil
	}

	parentState, err := search.derivePath(node.Parent)

	if err != nil {
		return nil, err
	}

	return parentState.ApplyAction(node.Action, search.rng)
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
backpropagate aggregates the economic rollout outcome up the tree with
Welford's online algorithm, maintaining the running mean and the sum of
squared deviations for the sample variance.
*/
func backpropagate(leaf *SearchNode, reward float64) {
	current := leaf

	for current != nil {
		current.Visits++
		delta := reward - current.Mean
		current.Mean += delta / float64(current.Visits)
		delta2 := reward - current.Mean
		current.SumSquaredDeviations += delta * delta2
		current = current.Parent
	}
}

/*
bestChild is the feasible action with the best search value under the
economic objective: highest mean economic reward, ties broken by visits. It
reports whether any child was actually visited, so the caller can represent
an unevaluated search explicitly instead of panicking.
*/
func bestChild(root *SearchNode) (*SearchNode, bool) {
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

	return best, best != nil
}

/*
alternativeEstimate returns the stored causal estimate for an action, or a
zero-valued estimate when the action was never estimated.
*/
func alternativeEstimate(alternatives []ActionEstimate, action Action) ActionEstimate {
	for _, estimate := range alternatives {
		if estimate.Action == action {
			return estimate
		}
	}

	return ActionEstimate{Action: action}
}

func (search *Search) trace(root *SearchNode, rootState State, alternatives []ActionEstimate) *Trace {
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
			Estimated:  alternativeEstimate(alternatives, child.Action),
		})
	}

	// Descending visits; equal visits are ordered deterministically by
	// Action so the branch order is stable and reproducible.
	slices.SortFunc(trace.Branches, func(left BranchTrace, right BranchTrace) int {
		if left.Visits != right.Visits {
			if left.Visits > right.Visits {
				return -1
			}

			return 1
		}

		if left.Action != right.Action {
			if left.Action < right.Action {
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
