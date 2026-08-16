package mcts

import "github.com/theapemachine/symm/nomagique/causal"

/*
CausalEngine supplies interventional and abductive estimates to tree search.
*/
type CausalEngine interface {
	DoExpectation(
		history [][]float64,
		target int,
		minimumRows int,
		treatment int,
		level float64,
		controls []int,
	) (float64, error)
	AbductiveCounterfactual(
		history [][]float64,
		target int,
		minimumRows int,
		features []int,
		linear bool,
		actualRow []float64,
		treatment int,
		level float64,
	) (counterfactual float64, noise float64, err error)
}

/*
DefaultCausalEngine delegates to the migrated causal table implementation.
*/
type DefaultCausalEngine struct{}

func (DefaultCausalEngine) DoExpectation(
	history [][]float64,
	target int,
	minimumRows int,
	treatment int,
	level float64,
	controls []int,
) (float64, error) {
	return causal.DoExpectation(
		history, target, minimumRows, treatment, level, controls,
	)
}

func (DefaultCausalEngine) AbductiveCounterfactual(
	history [][]float64,
	target int,
	minimumRows int,
	features []int,
	linear bool,
	actualRow []float64,
	treatment int,
	level float64,
) (counterfactual float64, noise float64, err error) {
	return causal.AbductiveCounterfactual(
		history,
		target,
		minimumRows,
		features,
		linear,
		actualRow,
		treatment,
		level,
	)
}
