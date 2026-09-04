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

	// Counterfactual provenance. These record how much of the branch's
	// value came from Pearl's third rung rather than real rollouts, so a
	// decision can be audited for how much of it was imagined.
	CounterfactualReward float64
	CounterfactualMass   float64
	CounterfactualMean   float64
	EffectiveVisits      float64
	BlendedValue         float64

	// CausalExpectation is the interventional estimate that biased
	// selection, defined only when the structural model identified it.
	CausalExpectation        float64
	CausalExpectationDefined bool

	// Pruned reports that the branch was causally rejected and withdrawn
	// from selection.
	Pruned bool
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

	// Causal is the optional structural model backing Pearl's second and
	// third rungs. When nil the search degrades to pure observational MCTS
	// rather than fabricating causal terms.
	Causal CausalEngine
	// CausalPolicy declares the columns, support floor, and weights the
	// causal queries run under. It is inert unless Causal is set.
	CausalPolicy CausalPolicy

	rng *rand.Rand
}

/*
CausalPolicy declares how the search consults the structural model. Every field
is explicit strategy policy or a structural fact about the observational table;
none is inferred from market evidence.
*/
type CausalPolicy struct {
	// TargetColumn is the index of the economic outcome in a state vector.
	TargetColumn int
	// TreatmentColumn is the index of the intervened quantity.
	TreatmentColumn int
	// ControlColumns are the backdoor adjustment set for the interventional
	// query: the confounders that must be held fixed for the estimate to
	// identify a causal effect rather than a correlation.
	ControlColumns []int
	// FeatureColumns are the explanatory columns the counterfactual fit
	// regresses the target on during abduction.
	FeatureColumns []int
	// MinimumRows is the observational support floor. Below it no causal
	// query runs, and the search stays purely observational.
	MinimumRows int
	// LinearFit selects a linear structural fit over regression stumps for
	// the counterfactual query.
	LinearFit bool
	// ExpectationWeight scales the interventional bias added to the selection
	// score. It carries reward units per unit of expectation and is declared
	// strategy policy, never derived from evidence. Zero disables the bias.
	ExpectationWeight float64
	// MaxCounterfactualMass caps the virtual visits one branch may accrue, so
	// counterfactual evidence can inform selection without ever drowning out
	// real rollout experience. Zero or negative leaves it uncapped.
	MaxCounterfactualMass float64
	// RejectionFloor is the dominance margin at which a branch is withdrawn
	// from selection, in the same units as the economic reward. A branch is
	// rejected when a live sibling's identified interventional expectation
	// exceeds its own by more than this margin.
	//
	// It is deliberately comparative rather than absolute. DoExpectation
	// returns E[wealth change | do(exposure)] in currency, not a normalized
	// score, so an absolute cutoff is scale-dependent and — worse — collapses
	// on a declining tape, where every action's expectation goes negative and
	// a fixed floor would condemn Wait and Exit alongside Enter. Asking
	// instead whether some other action is decisively better is invariant to
	// that common offset: on a falling market, Wait dominating Enter by $499
	// is exactly the signal to keep.
	//
	// Soft UCB bias cannot substitute for this. The exploration term grows
	// without bound as a branch goes unvisited, so UCB is mathematically
	// obliged to keep resampling an arm the causal model has already
	// condemned. Hard rejection is what stops that.
	RejectionFloor float64
	// RejectionEnabled arms the gate. It is explicit because zero is a
	// meaningful margin — reject any action another strictly dominates — and
	// must be distinguishable from "no rejection configured".
	RejectionEnabled bool
}

/*
causalReady reports whether the structural model may be consulted at all.
*/
func (search *Search) causalReady(history [][]float64) bool {
	return search.Causal != nil &&
		search.CausalPolicy.MinimumRows > 0 &&
		len(history) >= search.CausalPolicy.MinimumRows
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
SetSeed updates the search engine's deterministic seed and resets its random generator.
*/
func (search *Search) SetSeed(seed int64) {
	if search == nil {
		return
	}

	search.Seed = seed
	search.rng = rand.New(rand.NewSource(seed))
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

	// history is the empirical observational evidence the structural model fits
	// on, seeded from whatever the root state can supply. In accordance with
	// §8 (Simulation Is Not Observation), simulated rollout trajectories remain
	// hypothetical and are never appended to history or causal tables.
	history := rootHistory(rootState)

	for iteration := 0; iteration < search.Iterations; iteration++ {
		selected := search.selectNode(root, history)

		expanded, err := search.expandNode(selected)

		if err != nil {
			continue
		}

		reward, trajectory, err := search.simulate(expanded, estimator)

		if err != nil {
			continue
		}

		backpropagate(expanded, reward)
		search.counterfactualBackpropagate(expanded, trajectory, history)
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
	result.ExpectedEconomicOutcome = best.BlendedValue()
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
func (search *Search) selectNode(root *SearchNode, history [][]float64) *SearchNode {
	current := root

	for len(current.Children) > 0 && len(current.UntakenActions) == 0 {
		current = search.ucbChild(current, history)
	}

	return current
}

/*
ucbChild applies the explicit uncertainty-aware UCB selection rule, augmented
with Pearl's interventional rung:

	score(child) = value(child)
	             + C * sqrt(ln(N_parent + 1) / (n_child + 1))
	             + U * std(child) / sqrt(n_child + 1)
	             + W * E[reward | do(action = level)]

value(child) blends observed rollout reward with counterfactual evidence in
proportion to the mass behind each, so a branch that has only ever been
evaluated counterfactually still arrives with an estimate.

C is the policy exploration constant in reward units; U is the policy
uncertainty weight and is dimensionless (the standard error it scales already
carries reward units). W is the policy expectation weight.

The interventional term is added only when the structural model identifies an
effect for that action. A failed or unsupported query contributes nothing
rather than a fabricated zero, because zero is a meaningful expectation and
must not be confused with an absent one.
*/
func (search *Search) ucbChild(node *SearchNode, history [][]float64) *SearchNode {
	bestScore := math.Inf(-1)
	var best *SearchNode

	causalReady := search.causalReady(history)

	for _, child := range node.Children {
		// A causally rejected branch is never selected again. This is the
		// hard cutoff that soft score bias cannot provide.
		if child.Pruned {
			continue
		}

		score := child.BlendedValue() +
			search.ExplorationConstant*math.Sqrt(
				math.Log(node.EffectiveVisits()+1)/(child.EffectiveVisits()+1),
			) +
			search.UncertaintyWeight*child.RewardStandardDeviation()/math.Sqrt(float64(child.Visits+1))

		child.CausalExpectationDefined = false

		if causalReady {
			if expectation, defined := search.doExpectation(child, history); defined {
				child.CausalExpectation = expectation
				child.CausalExpectationDefined = true
				score += search.CausalPolicy.ExpectationWeight * expectation
			}
		}

		if score > bestScore {
			bestScore = score
			best = child
		}
	}

	if search.prune(node) {
		// Rejection withdrew at least one branch; reselect over survivors so
		// this iteration does not descend into a branch just condemned.
		return search.ucbChild(node, history)
	}

	if best == nil {
		// Every child is pruned. Selection still has to return a node, so
		// the least-condemned branch is reinstated rather than panicking:
		// an empty action set is a decision the caller makes from the
		// result, not a crash inside the search.
		return reinstate(node)
	}

	return best
}

/*
prune withdraws branches that a live sibling decisively dominates under the
structural model, and reports whether it withdrew any.

Domination is comparative: a branch is condemned only when the best identified
interventional expectation among its siblings exceeds its own by more than the
policy's margin. This is what keeps the gate safe on a declining tape. The
expectations are absolute wealth changes, so in a broad sell-off every action
scores negative; an absolute floor would reject Wait and Exit together with
Enter and leave the search with nothing. A margin between siblings is invariant
to that shared offset, so the gate still fires on the case that matters —
Enter being far worse than Wait — without ever condemning the safe action for
the market's overall direction.

Three guarantees hold regardless of policy:

Absence of evidence is never a veto. A branch whose expectation was not
identified is left to ordinary UCB exploration rather than condemned, because
a failed query says nothing about the action.

The best branch is never pruned. It is the reference the comparison is drawn
against, so it cannot be dominated by definition.

The survivor set is never emptied. If every candidate would be withdrawn, none
is, and selection proceeds normally.
*/
func (search *Search) prune(node *SearchNode) bool {
	if !search.CausalPolicy.RejectionEnabled {
		return false
	}

	best, identified := bestCausalExpectation(node)

	if !identified {
		// Nothing is identified, so there is no reference to dominate
		// against and nothing can be condemned.
		return false
	}

	margin := search.CausalPolicy.RejectionFloor
	condemned := make([]*SearchNode, 0, len(node.Children))
	survivors := 0

	for _, child := range node.Children {
		if child.Pruned {
			continue
		}

		// An unidentified branch is explored, never condemned.
		if !child.CausalExpectationDefined {
			survivors++
			continue
		}

		// The reference branch itself always survives.
		if child == best {
			survivors++
			continue
		}

		if best.CausalExpectation-child.CausalExpectation > margin {
			condemned = append(condemned, child)
			continue
		}

		survivors++
	}

	if survivors == 0 || len(condemned) == 0 {
		return false
	}

	for _, child := range condemned {
		child.Pruned = true
	}

	return true
}

/*
bestCausalExpectation returns the live branch carrying the highest identified
interventional expectation, and whether any branch identified one at all.
*/
func bestCausalExpectation(node *SearchNode) (*SearchNode, bool) {
	var best *SearchNode

	for _, child := range node.Children {
		if child.Pruned || !child.CausalExpectationDefined {
			continue
		}

		if best == nil || child.CausalExpectation > best.CausalExpectation {
			best = child
		}
	}

	return best, best != nil
}

/*
reinstate revives the least-condemned branch when rejection has withdrawn every
child, so selection always has somewhere to go. It prefers the highest
interventional expectation, since that is the evidence rejection acted on.
*/
func reinstate(node *SearchNode) *SearchNode {
	best, identified := bestCausalExpectation(node)

	if !identified {
		// Every child is pruned, so bestCausalExpectation found no live
		// branch. Fall back to the best expectation among the pruned.
		for _, child := range node.Children {
			if best == nil || child.CausalExpectation > best.CausalExpectation {
				best = child
			}
		}
	}

	if best == nil {
		panic("mcts: no selectable child")
	}

	best.Pruned = false

	return best
}

/*
doExpectation asks the structural model for E[reward | do(action)] under the
policy's backdoor adjustment set. It reports not-defined whenever the branch
declares no treatment level, the query fails, or the estimate is not finite.
*/
func (search *Search) doExpectation(child *SearchNode, history [][]float64) (float64, bool) {
	parentState := child.State

	if child.Parent != nil && child.Parent.State != nil {
		parentState = child.Parent.State
	}

	level, defined := interventionLevel(parentState, child.Action)

	if !defined {
		return 0, false
	}

	expectation, err := search.Causal.DoExpectation(
		history,
		search.CausalPolicy.TargetColumn,
		search.CausalPolicy.MinimumRows,
		search.CausalPolicy.TreatmentColumn,
		level,
		search.CausalPolicy.ControlColumns,
	)

	if err != nil || math.IsNaN(expectation) || math.IsInf(expectation, 0) {
		return 0, false
	}

	return expectation, true
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
completing a greedy rollout. It also returns the vectorized trajectory it
walked, which becomes observational evidence for the structural model.

A state that cannot vectorize itself yields an empty trajectory, which simply
leaves the causal layer without new evidence rather than failing the rollout. Re-deriving per visit is what makes the search
integrate over the stochastic transition distribution: a root action is not
frozen to the single first-step realization sampled at expansion time, and
each visit to the same action draws a new market realization. The random
source samples the market transition noise, so each trajectory walks a
plausible causal path.
*/
func (search *Search) simulate(
	node *SearchNode,
	estimator ActionEstimator,
) (float64, [][]float64, error) {
	state, err := search.derivePath(node)

	if err != nil {
		return 0, nil, err
	}

	var trajectory [][]float64
	trajectory = appendVector(trajectory, state)

	for !state.IsTerminal() {
		actions := state.GetPossibleActions()

		if len(actions) == 0 {
			break
		}

		action := search.rolloutAction(state, actions, estimator)
		next, err := state.ApplyAction(action, search.rng)

		if err != nil {
			return 0, nil, err
		}

		state = next
		trajectory = appendVector(trajectory, state)
	}

	return state.GetReward(), trajectory, nil
}

/*
appendVector appends a state's observational row when the state can produce
one. States that do not implement Vectorizer contribute no evidence.
*/
func appendVector(trajectory [][]float64, state State) [][]float64 {
	vectorizer, supported := state.(Vectorizer)

	if !supported {
		return trajectory
	}

	row := vectorizer.ToVector()

	if len(row) == 0 {
		return trajectory
	}

	for _, value := range row {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return trajectory
		}
	}

	return append(trajectory, row)
}

/*
rootHistory seeds the observational table from the root state when it can
supply prior evidence.
*/
func rootHistory(state State) [][]float64 {
	provider, supported := state.(HistoryProvider)

	if !supported {
		return nil
	}

	return provider.History()
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
economic objective: highest blended value, ties broken by effective visits.

The blended value carries counterfactual evidence, so a branch that Pearl's
third rung showed to be superior can win the decision rather than merely
attracting more exploration. Grounding is still required: a branch with no
real rollout is skipped regardless of how good its counterfactuals look.

It reports whether any child was actually visited, so the caller can
represent an unevaluated search explicitly instead of panicking.
*/
func bestChild(root *SearchNode) (*SearchNode, bool) {
	var best *SearchNode

	for _, child := range root.Children {
		// At least one real rollout is required. Counterfactual evidence
		// may rank a branch, but it may never be the sole grounds for
		// acting: an action that never survived a simulated trajectory is
		// imagination, and this decision spends real capital.
		if child.Visits == 0 {
			continue
		}

		if best == nil || child.BlendedValue() > best.BlendedValue() ||
			(child.BlendedValue() == best.BlendedValue() &&
				child.EffectiveVisits() > best.EffectiveVisits()) {
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
			Action:                   child.Action,
			Visits:                   child.Visits,
			MeanReward:               child.MeanReward(),
			RewardStd:                child.RewardStandardDeviation(),
			Estimated:                alternativeEstimate(alternatives, child.Action),
			CounterfactualReward:     child.CounterfactualReward,
			CounterfactualMass:       child.CounterfactualMass,
			CounterfactualMean:       child.CounterfactualMean(),
			EffectiveVisits:          child.EffectiveVisits(),
			BlendedValue:             child.BlendedValue(),
			CausalExpectation:        child.CausalExpectation,
			CausalExpectationDefined: child.CausalExpectationDefined,
			Pruned:                   child.Pruned,
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

/*
counterfactualBackpropagate is Pearl's third rung applied to the search tree.

For each node on the path just rolled out, every sibling branch the rollout did
not take is asked the counterfactual question: given the environmental noise
actually abducted from this trajectory, what outcome would this other action
have produced? The answer updates that sibling without ever rolling it out.

This is what makes a causal search cheaper than a plain one. A single rollout
yields evidence about every legal action at each decision node it passes, so
branch statistics converge in far fewer iterations than one-branch-per-rollout
backpropagation allows.

The virtual outcome is weighted by the precision the structural model derives
from its own reconstruction error: a counterfactual built on a poorly explained
factual row contributes proportionally less. Virtual mass accumulates in a
separate accumulator from real visits, so it can never contaminate the observed
reward mean or its Welford variance, and it is capped by policy so simulated
experience cannot outweigh real rollouts.
*/
func (search *Search) counterfactualBackpropagate(
	leaf *SearchNode,
	trajectory [][]float64,
	history [][]float64,
) {
	if !search.causalReady(history) || len(trajectory) == 0 {
		return
	}

	current := leaf
	trajectoryIndex := len(trajectory) - 1

	for current != nil && current.Parent != nil {
		if trajectoryIndex < 0 {
			return
		}

		actual := trajectory[trajectoryIndex]

		for _, sibling := range current.Parent.Children {
			if sibling == current {
				continue
			}

			search.counterfactualUpdate(sibling, actual, history)
		}

		current = current.Parent
		trajectoryIndex--
	}
}

/*
counterfactualUpdate applies one abduction-intervention-prediction cycle to a
single untaken sibling branch.
*/
func (search *Search) counterfactualUpdate(
	sibling *SearchNode,
	actual []float64,
	history [][]float64,
) {
	parentState := sibling.State

	if sibling.Parent != nil && sibling.Parent.State != nil {
		parentState = sibling.Parent.State
	}

	level, defined := interventionLevel(parentState, sibling.Action)

	if !defined {
		return
	}

	policy := search.CausalPolicy

	if policy.MaxCounterfactualMass > 0 &&
		sibling.CounterfactualMass >= policy.MaxCounterfactualMass {
		return
	}

	virtual, _, precision, err := search.Causal.AbductiveCounterfactual(
		history,
		policy.TargetColumn,
		policy.MinimumRows,
		policy.FeatureColumns,
		policy.LinearFit,
		actual,
		policy.TreatmentColumn,
		level,
	)

	if err != nil ||
		math.IsNaN(virtual) || math.IsInf(virtual, 0) ||
		math.IsNaN(precision) || math.IsInf(precision, 0) ||
		precision <= 0 {
		return
	}

	if precision > 1 {
		precision = 1
	}

	if policy.MaxCounterfactualMass > 0 {
		remaining := policy.MaxCounterfactualMass - sibling.CounterfactualMass

		if precision > remaining {
			precision = remaining
		}
	}

	sibling.CounterfactualPrecision = precision
	sibling.CounterfactualReward += virtual * precision
	sibling.CounterfactualMass += precision
}
