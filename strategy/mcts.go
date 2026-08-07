package strategy

import (
	"github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/types"
)

const (
	ActionNothing         float64 = 0.0
	ActionEnter           float64 = 1.0
	mctsMinimumCausalRows         = 12
	mctsSearchIterations          = 50
)

/*
StrategyState is the binary state evaluated by causal MCTS.

Treatment is the actual intervention level observed by the causal model.
Standing aside intervenes at zero. Both actions terminate immediately, so the
search compares the two do-expectations without adding a second decision rule.
*/
type StrategyState struct {
	Symbol      string
	Condition   float64
	Contagion   float64
	Treatment   float64
	Reward      float64
	Decided     bool
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
	}

	return ""
}

func (strategyState StrategyState) IsTerminal() bool {
	return strategyState.Decided
}

func (strategyState StrategyState) GetReward() float64 {
	return strategyState.Reward
}

func (strategyState StrategyState) GetPossibleActions() []float64 {
	if strategyState.Decided {
		return nil
	}

	return []float64{ActionNothing, ActionEnter}
}

func (strategyState StrategyState) ApplyAction(action float64) mcts.State {
	next := strategyState

	if strategyState.IsTerminal() {
		return next
	}

	next.Decided = true
	next.Reward = 0

	switch action {
	case ActionEnter:
		next.Treatment = strategyState.Treatment
	case ActionNothing:
		next.Treatment = 0
	}

	return next
}

func (strategyState StrategyState) ToVector() []float64 {
	return []float64{
		strategyState.Condition,
		strategyState.Contagion,
		strategyState.Treatment,
		strategyState.Reward,
	}
}

/*
GetInterventionLevel is the value the SCM's treatment variable is held at when
the search asks what an action would do.

Enter uses the actual treatment observed by the causal model. Do Not Enter uses
the user-defined standing-aside intervention do(0). The action enum itself is
never passed to the causal model as a treatment value.
*/
func (strategyState StrategyState) GetInterventionLevel(action float64) float64 {
	switch action {
	case ActionEnter:
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
