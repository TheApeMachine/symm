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

// StrategyState implements mcts.State for a symbol's decision trajectory.
type StrategyState struct {
	Symbol    string
	Energy    float64 // Control 1
	Surprise  float64 // Control 2
	Treatment float64 // Action / Expected Return
	Reward    float64 // Target / PnL
	Step      int
	MaxSteps  int
	IsHolding bool
}

func (s StrategyState) IsTerminal() bool {
	return s.Step >= s.MaxSteps || (s.IsHolding && s.Treatment < -0.02)
}

func (s StrategyState) GetReward() float64 {
	return s.Reward
}

func (s StrategyState) GetPossibleActions() []float64 {
	if s.IsHolding {
		return []float64{ActionHold, ActionExit}
	}
	return []float64{ActionNothing, ActionEnter}
}

func (s StrategyState) ApplyAction(action float64) mcts.State {
	next := s
	next.Step++

	switch action {
	case ActionEnter:
		next.IsHolding = true
		next.Reward += s.Treatment - 0.0026 // Net of fees
	case ActionHold:
		next.Reward += s.Treatment * 0.9 // Expected decay
	case ActionExit:
		next.IsHolding = false
		next.Reward -= 0.0026
	case ActionNothing:
		next.Reward += 0.0
	}

	return next
}

func (s StrategyState) ToVector() []float64 {
	return []float64{s.Energy, s.Surprise, s.Treatment, s.Reward}
}

// CausalEngineAdapter wraps causal.NodeTable to satisfy mcts.CausalEngine.
type CausalEngineAdapter struct{}

func NewCausalEngineAdapter() CausalEngineAdapter {
	return CausalEngineAdapter{}
}

func (a CausalEngineAdapter) DoExpectation(
	rows [][]float64, target, minRows, treatment int, level float64, controls []int,
) (float64, error) {
	table, err := causal.NewNodeTableWrapper(rows, target, minRows)
	if err != nil {
		return 0, err
	}
	return table.DoExpectation(treatment, level, controls...)
}

func (a CausalEngineAdapter) AbductiveCounterfactual(
	rows [][]float64, target, minRows int, features []int, linear bool, row []float64, treatment int, intervention float64,
) (float64, float64, error) {
	table, err := causal.NewNodeTableWrapper(rows, target, minRows)

	if err != nil {
		return 0, 0, err
	}

	_, cf, noise, err := table.AbductiveCounterfactual(features, linear, row, target, treatment, intervention)
	return cf, noise, err
}
