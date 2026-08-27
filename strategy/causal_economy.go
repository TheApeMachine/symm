package strategy

import (
	"fmt"
	"math"
	"math/rand"
	"slices"

	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/nomagique/relation"
)

/*
transitionOrder returns the schema market coordinates in topological DAG
order: every variable's structural parents precede it. Within a tier the
coordinates are ordered by relation.CompareCoordinate so the sequence is a
total order and replay runs consume the random stream identically. Cycles
(which the schema authoring forbids but a fit could in principle exhibit)
fall back to the total coordinate order rather than looping forever.
*/
func (state *CausalState) transitionOrder() []relation.Coordinate {
	var coordinates []relation.Coordinate

	if len(state.ActiveClosure) > 0 {
		coordinates = append([]relation.Coordinate(nil), state.ActiveClosure...)
	} else {
		coordinates = make([]relation.Coordinate, 0, len(state.Transitions))

		for coordinate := range state.Transitions {
			coordinates = append(coordinates, coordinate)
		}
	}

	// A total, deterministic coordinate order is the tie-break and the seed
	// for Kahn's algorithm; without it the order would inherit map iteration
	// order and break replay determinism.
	slices.SortFunc(coordinates, relation.CompareCoordinate)

	indexOf := make(map[relation.Coordinate]int, len(coordinates))

	for index, coordinate := range coordinates {
		indexOf[coordinate] = index
	}

	// Parent→child adjacency restricted to the present transition set.
	children := make([][]int, len(coordinates))

	for index, coordinate := range coordinates {
		transition := state.Transitions[coordinate]

		if transition == nil {
			continue
		}

		for _, parent := range transition.Parents {
			if parentIndex, found := indexOf[parent.Parent.Coordinate]; found {
				children[parentIndex] = append(children[parentIndex], index)
			}
		}
	}

	inDegree := make([]int, len(coordinates))

	for _, targets := range children {
		for _, targetIndex := range targets {
			inDegree[targetIndex]++
		}
	}

	ordered := make([]relation.Coordinate, 0, len(coordinates))

	// A deterministic Kahn's algorithm: always emit the lowest-index ready
	// vertex, so the sequence is a total order independent of map iteration.
	for len(ordered) < len(coordinates) {
		ready := -1

		for index := range coordinates {
			if inDegree[index] == 0 {
				ready = index
				break
			}
		}

		if ready == -1 {
			// A cycle remains. Emit the remaining coordinates in total
			// coordinate order and stop; the schema forbids cycles, so this
			// is a defensive convergence guarantee, not a routing decision.
			for index := range coordinates {
				if inDegree[index] > 0 {
					ordered = append(ordered, coordinates[index])
				}
			}

			break
		}

		inDegree[ready] = -1
		ordered = append(ordered, coordinates[ready])

		for _, childIndex := range children[ready] {
			if inDegree[childIndex] > 0 {
				inDegree[childIndex]--
			}
		}
	}

	return ordered
}

/*
causalMarketModel evolves the active causal dependency closure of the market
system one step forward: each variable in the query's active closure advances
through its fitted transition (self history plus active graph-informed lagged
parents), evolving multi-step rollouts without requiring unrelated market
variables to be identified.
*/
type causalMarketModel struct {
	state *CausalState
}

func (model *causalMarketModel) Step(current mcts.MarketState, random *rand.Rand) (mcts.MarketState, float64, float64, error) {
	if model == nil || model.state == nil || len(model.state.Transitions) == 0 {
		return current, 0, 0, fmt.Errorf("causal market model: transition set unavailable")
	}

	nextAt := current.At.Add(model.state.StepLag)
	next := mcts.MarketState{
		At:      nextAt,
		Current: make(map[relation.Coordinate]float64, len(current.Current)),
		History: make(map[relation.Coordinate][]mcts.MarketSample, len(current.History)),
	}

	for coordinate, value := range current.Current {
		next.Current[coordinate] = value
	}

	for coordinate, samples := range current.History {
		next.History[coordinate] = append([]mcts.MarketSample(nil), samples...)
	}

	// Iterate the active closure in topological DAG order (parents before children).
	coordinates := model.state.transitionOrder()

	for _, coordinate := range coordinates {
		transition := model.state.Transitions[coordinate]

		if transition == nil || transition.Status != causal.IdentificationIdentified {
			return current, 0, 0, fmt.Errorf(
				"causal market model: transition for %s is not identified",
				coordinate.ID(),
			)
		}

		expected, noise, defined := transition.Step(current)

		if !defined {
			return current, 0, 0, fmt.Errorf(
				"causal market model: transition for %s is not defined at the state",
				coordinate.ID(),
			)
		}

		value := expected

		if noise > 0 && random != nil {
			value += noise * random.NormFloat64()
		}

		next.Current[coordinate] = value
		next.History[coordinate] = append(next.History[coordinate], mcts.MarketSample{
			At:    nextAt,
			Value: value,
		})
	}

	outcome := model.state.OutcomeVariable.Coordinate
	logReturn, found := next.Current[outcome]

	if !found {
		return current, 0, 0, fmt.Errorf("causal market model: outcome coordinate missing")
	}

	outcomeNoise := 0.0

	if transition, found := model.state.Transitions[outcome]; found && transition.ResidualVariance > 0 {
		outcomeNoise = math.Sqrt(transition.ResidualVariance)
	}

	return next, logReturn, outcomeNoise, nil
}

/*
causalActionEstimator supplies the causal economic estimate for one action:
the expected change in net wealth under the action, evaluated along the
deterministic expected market path (no noise sampling). The estimate carries
the transition's residual uncertainty, identification status, and effective
support. An action whose required causal outcome is non-identifiable is
Undefined; it never receives a fabricated zero.
*/
type causalActionEstimator struct {
	state *CausalState
}

func (estimator *causalActionEstimator) EstimateAction(state mcts.State, action mcts.Action) mcts.ActionEstimate {
	if estimator == nil || estimator.state == nil ||
		estimator.state.Identification != causal.IdentificationIdentified {
		return mcts.ActionEstimate{
			Action:               action,
			IdentificationStatus: toMctsStatus(causal.IdentificationNotIdentifiable),
			Defined:              false,
		}
	}

	economic, supported := state.(*mcts.EconomicState)

	if !supported {
		return mcts.ActionEstimate{
			Action:               action,
			IdentificationStatus: toMctsStatus(causal.IdentificationUndefined),
			Defined:              false,
		}
	}

	current := economic
	next, err := current.ApplyAction(action, nil)

	if err != nil {
		return mcts.ActionEstimate{
			Action:               action,
			IdentificationStatus: toMctsStatus(causal.IdentificationInsufficientSupport),
			Defined:              false,
		}
	}

	for !next.IsTerminal() {
		nextState, applyErr := next.ApplyAction(mcts.Wait, nil)

		if applyErr != nil {
			// Preserve the last valid state and report the failure as an
			// explicit Undefined estimate instead of fabricating a value
			// from a nil state.
			return mcts.ActionEstimate{
				Action:               action,
				IdentificationStatus: toMctsStatus(causal.IdentificationInsufficientSupport),
				Defined:              false,
			}
		}

		next = nextState
	}

	outcomeTransition := estimator.state.Transitions[estimator.state.OutcomeVariable.Coordinate]
	noise := 0.0
	support := 0.0

	if outcomeTransition != nil {
		if outcomeTransition.ResidualVariance > 0 {
			noise = math.Sqrt(outcomeTransition.ResidualVariance)
		}

		support = outcomeTransition.EffectiveSupport
	}

	return mcts.ActionEstimate{
		Action:               action,
		ExpectedOutcome:      next.GetReward(),
		Uncertainty:          noise,
		IdentificationStatus: toMctsStatus(causal.IdentificationIdentified),
		Support:              support,
		Defined:              true,
	}
}

/*
toMctsStatus converts the strategy's own causal identification status into the
search primitive's status type. The two enums share value ordering; the mapping
is explicit so the boundary stays auditable.
*/
func toMctsStatus(status causal.IdentificationStatus) mcts.IdentificationStatus {
	switch status {
	case causal.IdentificationIdentified:
		return mcts.IdentificationIdentified
	case causal.IdentificationNotIdentifiable:
		return mcts.IdentificationNotIdentifiable
	case causal.IdentificationUnsupportedTreatment:
		return mcts.IdentificationUnsupportedTreatment
	case causal.IdentificationInsufficientRank:
		return mcts.IdentificationInsufficientRank
	case causal.IdentificationInsufficientSupport:
		return mcts.IdentificationInsufficientSupport
	default:
		return mcts.IdentificationUndefined
	}
}
