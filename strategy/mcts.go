package strategy

import (
	"math"

	"github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/mcts"
	"github.com/theapemachine/symm/types"
)

const (
	ActionNothing float64 = 0.0
	ActionEnter   float64 = 1.0
	ActionHold    float64 = 2.0
)

/*
StrategyState is the sequential trajectory state evaluated over a multi-horizon window.

Treatment is the intervention level observed by the causal SCM model.
Horizon describes the maximum trajectory depth (e.g., H = 5 steps).
Step tracks current search depth within the rollout.
CumulativeReward is the total discounted causal expectation accumulated over the trajectory.
CanEnter carries the admission check into the initial decision node.
*/
type StrategyState struct {
	Symbol           string
	Condition        float64
	Contagion        float64
	Treatment        float64
	Reward           float64
	CumulativeReward float64
	Horizon          int
	Step             int
	Decided          bool
	CanEnter         bool
	GraphReward      float64
	Precision        float64
}

/*
strategyAction maps search actions to published decisions.
*/
func strategyAction(action float64) types.Action {
	switch action {
	case ActionNothing:
		return types.ActionNothing
	case ActionEnter, ActionHold:
		return types.ActionEnter
	}

	return ""
}

func (strategyState StrategyState) IsTerminal() bool {
	if strategyState.Horizon <= 0 {
		return strategyState.Decided
	}

	return strategyState.Decided || strategyState.Step >= strategyState.Horizon
}

func (strategyState StrategyState) GetReward() float64 {
	return strategyState.CumulativeReward
}

func (strategyState StrategyState) GetPossibleActions() []float64 {
	if strategyState.IsTerminal() {
		return nil
	}

	if strategyState.Step == 0 {
		if !strategyState.CanEnter {
			return []float64{ActionNothing}
		}

		return []float64{ActionNothing, ActionEnter}
	}

	if strategyState.Treatment > 0 {
		return []float64{ActionNothing, ActionHold}
	}

	return []float64{ActionNothing}
}

func (strategyState StrategyState) ApplyAction(action float64) mcts.State {
	next := strategyState

	if strategyState.IsTerminal() {
		return next
	}

	next.Step++

	switch action {
	case ActionEnter:
		next.Treatment = strategyState.Treatment
		discount := math.Pow(0.95, float64(next.Step-1))
		stepReward := strategyState.GraphReward * strategyState.Precision * discount
		next.Reward = stepReward
		next.CumulativeReward += stepReward
	case ActionHold:
		discount := math.Pow(0.95, float64(next.Step-1))
		stepReward := strategyState.GraphReward * strategyState.Precision * discount
		next.Reward = stepReward
		next.CumulativeReward += stepReward
	case ActionNothing:
		next.Treatment = 0
		next.Reward = 0
		next.Decided = true
	}

	maxHorizon := next.Horizon

	if maxHorizon <= 0 {
		maxHorizon = 1
	}

	if next.Step >= maxHorizon {
		next.Decided = true
	}

	return next
}

func (strategyState StrategyState) ToVector() []float64 {
	return []float64{
		strategyState.Condition,
		strategyState.Contagion,
		strategyState.Treatment,
		strategyState.CumulativeReward,
	}
}

/*
GetInterventionLevel is the value the SCM's treatment variable is held at when
the search asks what an action would do.
*/
func (strategyState StrategyState) GetInterventionLevel(action float64) float64 {
	switch action {
	case ActionEnter, ActionHold:
		return strategyState.Treatment
	default:
		return 0.0
	}
}

/*
mctsBranches reports every root child the search actually explored.
*/
func mctsBranches(root *mcts.Node) []types.DecisionMCTSBranch {
	if root == nil {
		return nil
	}

	branches := make([]types.DecisionMCTSBranch, 0, len(root.Children))

	for _, child := range root.Children {
		if child == nil {
			continue
		}

		meanReward := 0.0

		if child.Visits > 0 {
			meanReward = child.TotalReward / float64(child.Visits)
		}

		branches = append(branches, types.DecisionMCTSBranch{
			Action:     strategyAction(child.Action),
			Visits:     child.Visits,
			MeanReward: meanReward,
		})
	}

	return branches
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
