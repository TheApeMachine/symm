package strategy

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/nomagique/relation"
)

/*
causalMarketModel evolves the market one step under the fitted causal
transition model. The market evolves independently of strategy actions; the
action mutates only portfolio variables. The outcome coordinate's expected
next value is the step's log return, so portfolio wealth moves with exposure.
*/
type causalMarketModel struct {
	state *CausalState
}

func (model *causalMarketModel) Step(current mcts.MarketState) (mcts.MarketState, float64, float64, error) {
	if model == nil || model.state == nil || model.state.Transition == nil {
		return current, 0, 0, fmt.Errorf("causal market model: transition unavailable")
	}

	expected, noise, defined := model.state.Transition.Step(current.Values)

	if !defined {
		return current, 0, 0, fmt.Errorf("causal market model: transition is not defined")
	}

	next := mcts.MarketState{
		At:     current.At.Add(model.state.Transition.SelfLag),
		Values: make(map[relation.Coordinate]float64, len(current.Values)),
	}

	for coordinate, value := range current.Values {
		next.Values[coordinate] = value
	}

	next.Values[model.state.OutcomeVariable.Coordinate] = expected

	return next, expected, noise, nil
}

/*
causalActionEstimator supplies the causal economic estimate for one action:
the expected change in net wealth under the action, evaluated along the
deterministic expected market path. An action whose required causal outcome
is non-identifiable is Undefined; it never receives a fabricated zero.
*/
type causalActionEstimator struct {
	state *CausalState
}

func (estimator *causalActionEstimator) EstimateAction(state mcts.State, action mcts.Action) mcts.ActionEstimate {
	if estimator == nil || estimator.state == nil ||
		estimator.state.Transition == nil ||
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
	next, err := current.ApplyAction(action)

	if err != nil {
		return mcts.ActionEstimate{
			Action:               action,
			IdentificationStatus: causal.IdentificationInsufficientSupport,
			Defined:              false,
		}
	}

	for !next.IsTerminal() {
		next, err = next.ApplyAction(mcts.Wait)

		if err != nil {
			break
		}
	}

	noise := 0.0

	if estimator.state.Transition.ResidualVariance > 0 {
		noise = math.Sqrt(estimator.state.Transition.ResidualVariance)
	}

	return mcts.ActionEstimate{
		Action:               action,
		ExpectedOutcome:      next.GetReward(),
		Uncertainty:          noise,
		IdentificationStatus: causal.IdentificationIdentified,
		Support:              estimator.state.Transition.EffectiveSupport,
		Defined:              true,
	}
}
