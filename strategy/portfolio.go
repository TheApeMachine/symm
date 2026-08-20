package strategy

import (
	"math"
	"math/rand"
	"slices"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/mcts"
	logicgraph "github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/types"
)

/*
portfolioLeg is one selectable candidate: a snapshot of the symbol's
directional evidence summary, its identifiable opportunity archetype, its
epistemic trust, and its position in the current desk.
*/
type portfolioLeg struct {
	Symbol          string
	Summary         logicgraph.OpportunitySummary
	Opportunity     logicgraph.OpportunityScore
	Trust           float64
	ReserveEligible bool
	Held            bool
}

/*
portfolioEnterReference returns the exact action value that enters one leg.
It exists so the planner can compare a decoded choice against the leg it
belonged to without re-deriving the encoding.
*/
func portfolioEnterReference(index int) float64 {
	return float64(index)*3 + portfolioEnterOffset
}

/*
portfolioExitReference returns the exact action value that exits one held leg.
*/
func portfolioExitReference(index int) float64 {
	return float64(index)*3 + portfolioExitOffset
}

/*
portfolioHoldReference returns the exact action value that keeps one leg flat.
*/
func portfolioHoldReference(index int) float64 {
	return float64(index)*3 + portfolioHoldOffset
}

/*
portfolioAction encodes (leg, intervention) pairs as a single float64 so a
node carries one action value while still addressing exactly one leg. The
value is legIndex*3 plus a kind offset; zero is the global done action.
*/
const (
	portfolioEnterOffset float64 = 1
	portfolioExitOffset  float64 = 2
	portfolioHoldOffset  float64 = 3
	portfolioDoneAction  float64 = 0
)

var (
	errStateNil           = errStrategyMissingState()
	errStateNoPaths       = errStrategyNoPaths()
	errStateUnknownAction = errStrategyUnknownAction()
	errStateAlreadyHeld   = errStrategyAlreadyHeld()
	errStateNotHeld       = errStrategyNotHeld()
)

/*
PortfolioState is the market state for one MCTS decision round. It implements
mcts.State over the complete candidate universe instead of over a single
symbol, so the tree search itself decides which positions to open, which to
leave flat, and which held lots have stopped earning their slot.
*/
type PortfolioState struct {
	legs         []portfolioLeg
	held         []bool
	slots        int
	reserveSlots int
	step         int
	maxSteps     int
	done         bool
	err          error
}

/*
NewPortfolioState builds a bounded multi-leg state. flat and held are copied so
an ApplyAction child never mutates the parent slice.
*/
func NewPortfolioState(
	legs []portfolioLeg,
	slots int,
	reserveSlots int,
) *PortfolioState {
	held := make([]bool, len(legs))

	for index := range legs {
		if legs[index].Held {
			held[index] = true
		}
	}

	// slots and reserveSlots are the desk's open lanes, which already account
	// for any held positions; the state must not subtract them twice.
	return &PortfolioState{
		legs:         slices.Clone(legs),
		held:         held,
		slots:        max(0, slots),
		reserveSlots: max(0, reserveSlots),
		maxSteps:     portfolioHorizon(legs),
	}
}

/*
portfolioHorizon bounds rollouts by the number of decisions available: every
leg can be decided at least once, and the trailing done action closes the path.
A search can never fold past the candidate count.
*/
func portfolioHorizon(legs []portfolioLeg) int {
	if len(legs) == 0 {
		return 1
	}

	return len(legs) + 1
}

func (state *PortfolioState) GetPossibleActions() []float64 {
	if state == nil || state.err != nil || state.IsTerminal() {
		return nil
	}

	actions := make([]float64, 0, len(state.legs)*2+1)

	for index := range state.legs {
		base := float64(index)

		if state.held[index] {
			actions = append(actions, base*3+portfolioExitOffset, base*3+portfolioHoldOffset)
			continue
		}

		if state.slots > 0 || (state.legs[index].ReserveEligible && state.reserveSlots > 0) {
			actions = append(actions, base*3+portfolioEnterOffset)
		}

		actions = append(actions, base*3+portfolioHoldOffset)
	}

	actions = append(actions, portfolioDoneAction)
	return actions
}

func (state *PortfolioState) IsTerminal() bool {
	return state == nil || state.err != nil || state.done ||
		(state.step >= state.maxSteps &&
			state.reserveSlots+state.slots <= 0)
}

func (state *PortfolioState) ToVector() []float64 {
	if state == nil {
		return nil
	}

	// The portfolio UCT has no vectorized causal table; the row satisfies the
	// State contract with the portfolio's own cumulative target while the
	// planner never feeds this back into the single-symbol SCM fit.
	return []float64{0, portfolioDoneAction, state.GetReward()}
}

func (state *PortfolioState) GetReward() float64 {
	if state == nil {
		return 0
	}

	reward := 0.0

	for index, leg := range state.legs {
		if !state.held[index] {
			continue
		}

		score := leg.Summary.Score
		trust := leg.Trust
		decay := 1 / math.Sqrt(float64(state.step+1))

		// A leg that never classified an opportunity still carries structural
		// evidence; its trust and lifecycle fall back to the summary, so the
		// search does not silently zero a candidate the planner admitted.
		if trust <= 0 {
			trust = leg.Summary.Confidence
		}

		lifecycleWeight := 1.0

		if leg.Opportunity.Type != "" {
			lifecycleWeight = opportunityLifecycleWeight(leg.Opportunity.Lifecycle)
		}

		reward += score * trust * lifecycleWeight * decay
	}

	return reward
}

/*
opportunityLifecycleWeight states how much an open slot is worth in each
lifecycle phase. Confirming and Accelerating are the entry windows the system
may hold; Climax still carries value but no longer compounds; Emergent and
Exhausting earn nothing because the evidence is not yet coherent or has
already decayed.
*/
func opportunityLifecycleWeight(
	lifecycle types.OpportunityLifecycle,
) float64 {
	switch lifecycle {
	case types.LifecycleConfirming:
		return 0.6
	case types.LifecycleAccelerating:
		return 1
	case types.LifecycleClimax:
		return 0.4
	default:
		return 0
	}
}

func (state *PortfolioState) ApplyAction(action float64) mcts.State {
	if state == nil {
		return &PortfolioState{err: errStateNil}
	}

	child := &PortfolioState{
		legs:         state.legs,
		held:         slices.Clone(state.held),
		slots:        state.slots,
		reserveSlots: state.reserveSlots,
		step:         state.step + 1,
		maxSteps:     state.maxSteps,
		done:         state.done,
		err:          state.err,
	}

	if child.err != nil {
		return child
	}

	if action == portfolioDoneAction {
		child.done = true
		return child
	}

	index := int(math.Floor((action - 1) / 3))

	if index < 0 || index >= len(child.legs) {
		child.err = errStateUnknownAction
		return child
	}

	kind := math.Mod(action, 3)

	if kind == 0 {
		kind = 3
	}
	leg := child.legs[index]

	switch kind {
	case portfolioEnterOffset:
		if child.held[index] {
			child.err = errStateAlreadyHeld
			return child
		}

		if leg.ReserveEligible && child.reserveSlots > 0 {
			child.reserveSlots--
			child.held[index] = true
			return child
		}

		if child.slots > 0 {
			child.slots--
			child.held[index] = true
			return child
		}
	case portfolioExitOffset:
		if !child.held[index] {
			child.err = errStateNotHeld
			return child
		}

		child.held[index] = false
	case portfolioHoldOffset:
	default:
		child.err = errStateUnknownAction
	}

	return child
}

/*
portfolioSearch runs a UCT tree search over the candidate universe. The
treatment-intervention causal model was solved per symbol upstream by the
evidence graph; this search solves the remaining portfolio problem those
summaries cannot answer by themselves: which positions earn the scarce
slots, and which held lots are now worth less than a flat candidate.
The root is returned with the principal variation marked by most-visited
branches, not UCT scores, because the round's decision is the action the
search spent its visit budget proving, not the one still under exploration.
*/
func portfolioSearch(rootState *PortfolioState, iterations int) (*mcts.Node, error) {
	if rootState == nil {
		return nil, errStateNil
	}

	if iterations < 1 {
		iterations = 1
	}

	root := &mcts.Node{
		State:          rootState,
		UntakenActions: slices.Clone(rootState.GetPossibleActions()),
	}
	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	for iteration := 0; iteration < iterations; iteration++ {
		selected := selectPortfolioNode(root)
		expanded := expandPortfolioNode(selected)

		reward, err := rolloutPortfolio(expanded, random)

		if err != nil {
			return nil, err
		}

		backpropagatePortfolio(expanded, reward)
	}

	if len(root.Children) == 0 {
		return nil, errStateNoPaths
	}

	markPortfolioPrincipal(root)
	return root, nil
}

/*
markPortfolioPrincipal walks the most-visited branch at every depth so the
principal variation reports the provably explored decision, not a UCT tie.
*/
func markPortfolioPrincipal(root *mcts.Node) {
	current := root

	for current != nil {
		current.Principal = true

		if len(current.Children) == 0 {
			return
		}

		current = mostVisitedPortfolioChild(current)
	}
}

func mostVisitedPortfolioChild(node *mcts.Node) *mcts.Node {
	var best *mcts.Node

	for _, child := range node.Children {
		if best == nil || child.Visits > best.Visits {
			best = child
			continue
		}

		if child.Visits == best.Visits &&
			child.MeanReward() > best.MeanReward() {
			best = child
		}
	}

	if best == nil {
		panic("strategy: no visited portfolio branch")
	}

	return best
}

func selectPortfolioNode(node *mcts.Node) *mcts.Node {
	current := node

	for len(current.Children) > 0 && len(current.UntakenActions) == 0 {
		current = bestPortfolioChild(current)
	}

	return current
}

func bestPortfolioChild(node *mcts.Node) *mcts.Node {
	var best *mcts.Node
	bestScore := math.Inf(-1)

	for _, child := range node.Children {
		score := child.MeanReward() +
			math.Sqrt2*math.Sqrt(math.Log(float64(node.Visits)+1)/(float64(child.Visits)+1))

		if score > bestScore {
			bestScore = score
			best = child
		}
	}

	if best == nil {
		panic("strategy: no selectable portfolio branch")
	}

	return best
}

func expandPortfolioNode(node *mcts.Node) *mcts.Node {
	if len(node.UntakenActions) == 0 {
		return node
	}

	action := node.UntakenActions[len(node.UntakenActions)-1]
	node.UntakenActions = node.UntakenActions[:len(node.UntakenActions)-1]
	nextState := node.State.ApplyAction(action)
	child := &mcts.Node{
		State:          nextState,
		Action:         action,
		Parent:         node,
		UntakenActions: slices.Clone(nextState.GetPossibleActions()),
		Depth:          node.Depth + 1,
	}
	node.Children = append(node.Children, child)
	return child
}

func rolloutPortfolio(node *mcts.Node, random *rand.Rand) (float64, error) {
	current := node.State

	for !current.IsTerminal() {
		actions := current.GetPossibleActions()

		if len(actions) == 0 {
			break
		}

		current = current.ApplyAction(actions[random.Intn(len(actions))])
	}

	return current.GetReward(), nil
}

func backpropagatePortfolio(leaf *mcts.Node, reward float64) {
	current := leaf

	for current != nil {
		current.Visits++
		current.TotalReward += reward
		current = current.Parent
	}
}

func errStrategyMissingState() error {
	return errnie.Error(errnie.Err(
		errnie.Validation,
		"strategy: portfolio state required",
		nil,
	))
}

func errStrategyNoPaths() error {
	return errnie.Error(errnie.Err(
		errnie.Validation,
		"strategy: portfolio search explored no paths",
		nil,
	))
}

func errStrategyUnknownAction() error {
	return errnie.Error(errnie.Err(
		errnie.Validation,
		"strategy: portfolio action does not address a leg",
		nil,
	))
}

func errStrategyAlreadyHeld() error {
	return errnie.Error(errnie.Err(
		errnie.Validation,
		"strategy: cannot re-enter a held portfolio leg",
		nil,
	))
}

func errStrategyNotHeld() error {
	return errnie.Error(errnie.Err(
		errnie.Validation,
		"strategy: cannot exit a flat portfolio leg",
		nil,
	))
}
