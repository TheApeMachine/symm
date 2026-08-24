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
the parents after the first prediction. Each transition is evaluated with the
same as-of lag semantics it was fitted with (the state's timestamped
trajectory supplies the parent value valid at the required cutoff), and the
sampled next values are appended to the trajectory.

A market variable present in the state whose transition is not identified
makes the step unavailable: its future is genuinely unknown, and it must not
be silently carried forward as a persistence model. The random source
samples each transition's residual noise, so rollouts walk a distribution of
plausible causal trajectories. Actions never mutate market state: they
change portfolio variables only.
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
		present := false

		if _, found := current.Current[coordinate]; found {
			present = true
		}

		// An unidentified transition for a coordinate present in the state
		// is an unavailable future, not a silent persistence carry-forward.
		if transition == nil || transition.Status != causal.IdentificationIdentified {
			if present {
				return current, 0, 0, fmt.Errorf(
					"causal market model: transition for %s is not identified",
					coordinate.ID(),
				)
			}

			continue
		}

		expected, noise, defined := transition.Step(current)

		if !defined {
			if present {
				return current, 0, 0, fmt.Errorf(
					"causal market model: transition for %s is not defined at the state",
					coordinate.ID(),
				)
			}

			continue
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
