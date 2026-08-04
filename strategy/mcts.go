package strategy

import (
	"github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/mcts"
)

const (
	ActionNothing float64 = 0.0
	ActionEnter   float64 = 1.0
	ActionHold    float64 = 2.0
	ActionExit    float64 = 3.0
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
	Symbol string
	Energy float64 // Control 1

	Surprise float64 // Control 2

	// Treatment is the forecast return as a fraction of the reference price,
	// and RoundTripCost is the modelled friction of entering and exiting on
	// the same scale.
	Treatment     float64
	RoundTripCost float64

	Reward    float64 // Target / PnL
	Step      int
	MaxSteps  int
	IsHolding bool
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

func (strategyState StrategyState) IsTerminal() bool {
	return strategyState.Step >= strategyState.MaxSteps ||
		(strategyState.IsHolding && strategyState.Treatment < strategyState.reversalFraction())
}

func (strategyState StrategyState) GetReward() float64 {
	return strategyState.Reward
}

func (strategyState StrategyState) GetPossibleActions() []float64 {
	if strategyState.IsHolding {
		return []float64{ActionHold, ActionExit}
	}

	return []float64{ActionNothing, ActionEnter}
}

func (strategyState StrategyState) ApplyAction(action float64) mcts.State {
	next := strategyState
	next.Step++

	switch action {
	case ActionEnter:
		// Entering pays the whole round trip up front, because a position
		// opened in this rollout has to be closed within it to be worth
		// anything. The cost is the candidate's own modelled friction rather
		// than a constant, so a wide or expensive market prices itself out
		// instead of being judged against a figure taken from some other one.
		next.IsHolding = true
		next.Reward += strategyState.Treatment - strategyState.RoundTripCost
	case ActionHold:
		// A held forecast decays toward the horizon it was drawn for, so a
		// further step of holding is worth less than the forecast claims.
		next.Reward += strategyState.Treatment * 0.9
	case ActionExit:
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
	return []float64{strategyState.Energy, strategyState.Surprise, strategyState.Treatment, strategyState.Reward}
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

Each action instead names the forecast level it actually commits to: entering
takes the candidate's own expected return, holding takes what a further step of
it is worth after decay, and standing aside or closing out takes nothing. Those
are levels the treatment column genuinely carries, so the interventional
expectation is read from inside the model's support.
*/
func (strategyState StrategyState) GetInterventionLevel(action float64) float64 {
	switch action {
	case ActionEnter:
		return strategyState.Treatment
	case ActionHold:
		return strategyState.Treatment * 0.9
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
