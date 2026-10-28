package mcts

import "github.com/theapemachine/symm/nomagique/causal"

// State represents the environment interface that MCTS interacts with.
type State interface {
	IsTerminal() bool
	GetReward() float64
	GetPossibleActions() []float64
	ApplyAction(action float64) State
	ToVector() []float64 // Converts the state to the row format expected by causal.Table
}

// InterventionMapper maps a discrete action onto the level the SCM's treatment
// variable is held at to represent it.
//
// Actions are an enum and a treatment is a measured quantity, so the two share
// a scale only by accident. A State that implements this names the level each
// action corresponds to; one that does not is intervened on with the action
// value itself, which is the historical behaviour.
type InterventionMapper interface {
	GetInterventionLevel(action float64) float64
}

// interventionLevel is the treatment level representing the action that
// produced a node.
func interventionLevel(node *Node) float64 {
	if mapper, ok := node.State.(InterventionMapper); ok {
		return mapper.GetInterventionLevel(node.Action)
	}

	return node.Action
}

// CausalEngine abstracts your custom causal package's analytical methods.
// This interface allows us to pass wrapped versions of your private package methods.
type CausalEngine interface {
	DoExpectation(
		rows [][]float64,
		target, minRows,
		treatment int,
		level float64,
		controls []int,
	) (float64, error)

	AbductiveCounterfactual(
		rows [][]float64,
		target, minRows int,
		features []int,
		linear bool,
		row []float64,
		treatment int,
		intervention float64,
	) (float64, float64, error)
}

/*
DefaultCausalEngine evaluates MCTS history with nomagique's causal models.
*/
type DefaultCausalEngine struct{}

func (defaultCausalEngine DefaultCausalEngine) DoExpectation(
	rows [][]float64,
	target, minRows, treatment int,
	level float64,
	controls []int,
) (float64, error) {
	table, err := causal.NewNodeTableWrapper(rows, target, minRows)

	if err != nil {
		return 0, err
	}

	return table.DoExpectation(treatment, level, controls...)
}

func (defaultCausalEngine DefaultCausalEngine) AbductiveCounterfactual(
	rows [][]float64,
	target, minRows int,
	features []int,
	linear bool,
	row []float64,
	treatment int,
	intervention float64,
) (float64, float64, error) {
	table, err := causal.NewNodeTableWrapper(rows, target, minRows)

	if err != nil {
		return 0, 0, err
	}

	_, counterfactual, noise, err := table.AbductiveCounterfactual(
		features, linear, row, target, treatment, intervention,
	)

	return counterfactual, noise, err
}
