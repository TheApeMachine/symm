package strategy

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/nomagique/relation"
)

/*
causalMarketModel evolves the whole time-sliced market system one step
forward: every schema market variable advances through its own fitted
transition (self history plus schema-authorized, graph-informed lagged
parents), so the causal chain — e.g. Liquidity(t) → Flow(t+1) → Price(t+2),
Hawkes(t) → Flow(t+1) — unfolds over multi-step rollouts instead of freezing
the parents after the first prediction.

The random source samples each transition's residual noise, so rollouts walk
a distribution of plausible causal trajectories. The priced outcome movement
of the step is the outcome variable's next value. Actions never mutate market
state: they change portfolio variables only.
*/
type causalMarketModel struct {
	state *CausalState
}

func (model *causalMarketModel) Step(current mcts.MarketState, random *rand.Rand) (mcts.MarketState, float64, float64, error) {
	if model == nil || model.state == nil || len(model.state.Transitions) == 0 {
		return current, 0, 0, fmt.Errorf("causal market model: transition set unavailable")
	}

	next := mcts.MarketState{
		At:     current.At.Add(model.state.StepLag),
		Values: make(map[relation.Coordinate]float64, len(current.Values)+len(model.state.Transitions)),
	}

	for coordinate, value := range current.Values {
		next.Values[coordinate] = value
	}

	// Iterate the transition set in deterministic coordinate order so the
	// sampled rollouts consume the random stream identically across replay
	// runs (map iteration order is random).
	coordinates := make([]relation.Coordinate, 0, len(model.state.Transitions))

	for coordinate := range model.state.Transitions {
		coordinates = append(coordinates, coordinate)
	}

	sort.Slice(coordinates, func(left int, right int) bool {
		return coordinates[left].ID() < coordinates[right].ID()
	})

	for _, coordinate := range coordinates {
		transition := model.state.Transitions[coordinate]

		// A market variable whose transition is not yet identified is
		// carried at its last observed level: it is neither extrapolated
		// nor fabricated. Once its transition is fitted it joins the
		// evolving system.
		if transition == nil || transition.Status != causal.IdentificationIdentified {
			continue
		}

		expected, noise, defined := transition.Step(current.Values)

		if !defined {
			continue
		}

		value := expected

		if noise > 0 && random != nil {
			value += noise * random.NormFloat64()
		}

		next.Values[coordinate] = value
	}

	outcome := model.state.OutcomeVariable.Coordinate
	logReturn, found := next.Values[outcome]

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
			IdentificationStatus: causal.IdentificationNotIdentifiable,
			Defined:              false,
		}
	}

	economic, supported := state.(*mcts.EconomicState)

	if !supported {
		return mcts.ActionEstimate{
			Action:               action,
			IdentificationStatus: causal.IdentificationUndefined,
			Defined:              false,
		}
	}

	current := economic
	next, err := current.ApplyAction(action, nil)

	if err != nil {
		return mcts.ActionEstimate{
			Action:               action,
			IdentificationStatus: causal.IdentificationInsufficientSupport,
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
				IdentificationStatus: causal.IdentificationInsufficientSupport,
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
		IdentificationStatus: causal.IdentificationIdentified,
		Support:              support,
		Defined:              true,
	}
}
