package strategy

import (
	"math"

	"github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/types"
)

const (
	ActionNothing            float64 = 0.0
	ActionEnter              float64 = 1.0
	ActionHold               float64 = 2.0
	ActionCompleteTrajectory float64 = 3.0
	mctsMinimumCausalRows            = 12
	mctsSearchIterations             = 50
)

/*
StrategyState implements mcts.State for a symbol's decision trajectory.

Every field on the trajectory is a fraction of the reference price rather than
an amount of quote currency. The rollout compares a forecast against a friction
cost and against a reversal threshold, and neither comparison means anything
unless all three are on one scale: a currency return of 0.064 on a hundred
dollar symbol is the same trade as 0.00064 on a one dollar symbol, and a
threshold that reads one as worth taking and the other as noise is deciding by
the symbol's price rather than by its move.
*/
type StrategyState struct {
	Symbol               string
	Energy               float64
	Surprise             float64
	Treatment            float64
	Forecast             *types.ResonanceForecast
	RoundTripCost        float64
	HoldDiscount         float64
	HawkesSpectralRadius float64
	Reward               float64
	Step                 int
	IsHolding            bool
}

/*
Trace describes the exact root state and search budget handed to CausalMCTS.
The mutable result fields are filled by the evaluator after Search returns.
*/
func (strategyState StrategyState) Trace(
	causalRows, minimumRows, iterations int,
) types.DecisionMCTSTrace {
	horizonSteps := 0

	if strategyState.Forecast != nil {
		horizonSteps = strategyState.Forecast.SupportedHorizon
	}

	return types.DecisionMCTSTrace{
		Energy:               strategyState.Energy,
		Surprise:             strategyState.Surprise,
		Treatment:            strategyState.Treatment,
		RoundTripCost:        strategyState.RoundTripCost,
		HoldDiscount:         strategyState.HoldDiscount,
		HawkesSpectralRadius: strategyState.HawkesSpectralRadius,
		HoldPropagation:      strategyState.holdPropagation(),
		CausalRows:           causalRows,
		MinimumCausalRows:    minimumRows,
		Iterations:           iterations,
		HorizonSteps:         horizonSteps,
		Searchable:           causalRows >= minimumRows,
	}
}

/*
strategyAction translates only the search actions the live planner is allowed
to publish. Internal trajectory completion and unknown results remain absent.
*/
func strategyAction(action float64) types.Action {
	switch action {
	case ActionNothing:
		return types.ActionNothing
	case ActionEnter:
		return types.ActionEnter
	case ActionHold:
		return types.ActionHold
	}

	return ""
}

/*
reversalFraction is how far a held forecast must turn against the position
before the rollout abandons the branch, as a fraction of the reference price.

A held position already paid to enter, so the bar for abandoning it is a move
that exceeds what a round trip costs to unwind and re-establish rather than any
adverse move at all.
*/
func (strategyState StrategyState) reversalFraction() float64 {
	return -2.0 * strategyState.RoundTripCost
}

/*
holdPropagation combines the exhaustion survival probability with the expected
population carried into the next Hawkes generation. For branching matrix
spectral radius rho, one parent plus its expected first-generation offspring is
1+rho. Applying that population to the signed forecast lets a reflexive path
expand when propagation outweighs exhaustion, while the fitted model remains
stationary and the model-derived rollout horizon keeps the search finite.
*/
func (strategyState StrategyState) holdPropagation() float64 {
	if strategyState.HoldDiscount <= 0 || strategyState.HoldDiscount >= 1 ||
		strategyState.HawkesSpectralRadius < 0 ||
		strategyState.HawkesSpectralRadius >= 1 {
		return 0
	}

	propagation := strategyState.HoldDiscount * (1 + strategyState.HawkesSpectralRadius)

	if math.IsNaN(propagation) || math.IsInf(propagation, 0) {
		return 0
	}

	return propagation
}

func (strategyState StrategyState) IsTerminal() bool {
	return strategyState.Forecast == nil ||
		strategyState.Step >= strategyState.Forecast.SupportedHorizon ||
		(strategyState.IsHolding && strategyState.Treatment < strategyState.reversalFraction())
}

func (strategyState StrategyState) GetReward() float64 {
	return strategyState.Reward
}

func (strategyState StrategyState) GetPossibleActions() []float64 {
	if strategyState.IsHolding {
		return []float64{ActionHold, ActionCompleteTrajectory}
	}

	return []float64{ActionNothing, ActionEnter}
}

func (strategyState StrategyState) ApplyAction(action float64) mcts.State {
	next := strategyState

	if strategyState.IsTerminal() {
		return next
	}

	forecastStep, supported := strategyState.Forecast.Step(strategyState.Step)

	if !supported {
		next.Step = strategyState.Forecast.SupportedHorizon

		return next
	}

	next.Step++

	switch action {
	case ActionEnter:
		// Entering pays the whole round trip up front, because a position
		// opened in this rollout has to be closed within it to be worth
		// anything. The cost is the candidate's own modelled friction rather
		// than a constant, so a wide or expensive market prices itself out
		// instead of being judged against a figure taken from some other one.
		next.Treatment = forecastStep
		next.IsHolding = true
		next.Reward += next.Treatment - strategyState.RoundTripCost
	case ActionHold:
		// Each hold reads the next confidence-supported curve step rather than
		// reusing the first prediction. Exhaustion/Hawkes propagation still prices
		// how much of that step survives through the corresponding generation.
		next.Treatment = forecastStep * math.Pow(
			strategyState.holdPropagation(),
			float64(strategyState.Step),
		)

		next.Reward += next.Treatment
	case ActionCompleteTrajectory:
		// The round trip was already charged on entry, so exiting adds
		// nothing further. Charging it again would make every completed
		// trajectory pay twice and bias the search toward never opening.
		next.IsHolding = false
	case ActionNothing:
		next.Reward += 0.0
	}

	return next
}

func (strategyState StrategyState) ToVector() []float64 {
	return []float64{
		strategyState.Energy,
		strategyState.Surprise,
		strategyState.Treatment,
		strategyState.Reward,
	}
}

/*
GetInterventionLevel is the value the SCM's treatment variable is held at when
the search asks what an action would do.

The treatment column is the resonance stage's expected return, a fraction of the
reference price of order a thousandth. An action is an enum, so intervening with
the action itself asks the model what happens when the expected return is one or
three — a hundred or three hundred percent, hundreds of times outside anything
the model was fitted on. The answer to that question is an extrapolation, and it
arrives scaled far above the UCT terms it is added to, so it decides selection by
itself.

Each action instead names the forecast level it committed to in this state.
ApplyAction has already advanced a held treatment through its next Hawkes
generation before MCTS constructs the child node, so both entry and hold expose
the state's treatment directly. Standing aside or completing a rollout takes
nothing. Those are levels the treatment column genuinely carries, so the
interventional expectation is read from inside the model's support.
*/
func (strategyState StrategyState) GetInterventionLevel(action float64) float64 {
	switch action {
	case ActionEnter, ActionHold:
		return strategyState.Treatment
	default:
		return 0.0
	}
}

// CausalEngineAdapter wraps causal.NodeTable to satisfy mcts.CausalEngine.
type CausalEngineAdapter struct{}

func NewCausalEngineAdapter() CausalEngineAdapter {
	return CausalEngineAdapter{}
}

func (causalEngineAdapter CausalEngineAdapter) DoExpectation(
	rows [][]float64, target, minRows, treatment int, level float64, controls []int,
) (float64, error) {
	table, err := causal.NewNodeTableWrapper(rows, target, minRows)

	if err != nil {
		return 0, err
	}

	return table.DoExpectation(treatment, level, controls...)
}

func (causalEngineAdapter CausalEngineAdapter) AbductiveCounterfactual(
	rows [][]float64,
	target, minRows int,
	features []int,
	linear bool,
	row []float64,
	treatment int,
	intervention float64,
) (float64, float64, error) {
	table, err := causal.NewNodeTableWrapper(
		rows, target, minRows,
	)

	if err != nil {
		return 0, 0, err
	}

	_, cf, noise, err := table.AbductiveCounterfactual(
		features, linear, row, target, treatment, intervention,
	)

	return cf, noise, err
}
