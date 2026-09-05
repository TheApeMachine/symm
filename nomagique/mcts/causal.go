package mcts

import (
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/causal"
)

/*
InterventionMapper maps a discrete Action onto the level the structural model's
treatment variable is held at to represent it.

Actions are an enum and a treatment is a measured quantity, so the two share a
scale only by accident. A State that implements this names the level each action
corresponds to; a State that does not is not intervened on at all, because
silently treating the enum ordinal as a treatment level would fabricate a
quantity the causal model never observed.
*/
type InterventionMapper interface {
	GetInterventionLevel(action Action) (level float64, defined bool)
}

/*
interventionLevel resolves the treatment level representing one action. It
reports not-defined when the state declares no mapping, so the caller skips the
causal query instead of intervening at a meaningless level.
*/
func interventionLevel(state State, action Action) (float64, bool) {
	mapper, supported := state.(InterventionMapper)

	if !supported {
		return 0, false
	}

	return mapper.GetInterventionLevel(action)
}

/*
CausalEngine is the search's boundary onto Pearl's second and third rungs. It is
an interface so the search stays a leaf package: the engine can be the real
structural model, a stub, or nil.

DoExpectation answers the interventional question E[target | do(treatment=level)]
by backdoor standardization over the observed control distribution.

AbductiveCounterfactual answers the counterfactual question: given the factual
row actually observed, what would the target have been had the treatment been
set to level instead? It returns the reconstructed outcome, the abducted noise
term, and a bounded precision derived from the reconstruction error.
*/
type CausalEngine interface {
	DoExpectation(
		rows [][]float64,
		target int,
		minimumRows int,
		treatment int,
		level float64,
		controls []int,
	) (float64, error)

	AbductiveCounterfactual(
		rows [][]float64,
		target int,
		minimumRows int,
		features []int,
		linear bool,
		actual []float64,
		treatment int,
		level float64,
	) (counterfactual float64, noise float64, precision float64, err error)
}

type tableCacheEntry struct {
	table   *causal.Table
	rowsPtr *float64
	rowsLen int
	target  int
	minRows int
	linear  bool
}

var causalTableCache struct {
	sync.Mutex
	entry tableCacheEntry
}

func getOrNewTable(
	rows [][]float64,
	target int,
	minimumRows int,
	linear bool,
) (*causal.Table, error) {
	if len(rows) > 0 && len(rows[0]) > 0 {
		causalTableCache.Lock()
		firstElem := &rows[0][0]

		if causalTableCache.entry.table != nil &&
			causalTableCache.entry.rowsPtr == firstElem &&
			causalTableCache.entry.rowsLen == len(rows) &&
			causalTableCache.entry.target == target &&
			causalTableCache.entry.minRows == minimumRows &&
			causalTableCache.entry.linear == linear {
			table := causalTableCache.entry.table
			causalTableCache.Unlock()

			return table, nil
		}

		causalTableCache.Unlock()
	}

	table, err := causal.NewTable(rows, target, minimumRows, linear)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.PreconditionFailed,
			"[mcts] causal table construction failed",
			err,
		))
	}

	if len(rows) > 0 && len(rows[0]) > 0 {
		causalTableCache.Lock()
		causalTableCache.entry = tableCacheEntry{
			table:   table,
			rowsPtr: &rows[0][0],
			rowsLen: len(rows),
			target:  target,
			minRows: minimumRows,
			linear:  linear,
		}
		causalTableCache.Unlock()
	}

	return table, nil
}

/*
DefaultCausalEngine evaluates search history with the nomagique causal Table.
It reuses a cached table across queries when the history slice is identical,
avoiding repetitive deep-cloning and linear matrix inversions during search.
*/
type DefaultCausalEngine struct {
	// Linear selects the linear structural fit over regression stumps for the
	// interventional query. The counterfactual query takes its own flag from
	// the search policy, so both fits are explicit rather than implied.
	Linear bool
}

/*
DoExpectation standardizes the interventional expectation over observed controls.
*/
func (engine DefaultCausalEngine) DoExpectation(
	rows [][]float64,
	target int,
	minimumRows int,
	treatment int,
	level float64,
	controls []int,
) (float64, error) {
	table, err := getOrNewTable(rows, target, minimumRows, engine.Linear)

	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.PreconditionFailed,
			"[mcts] causal table construction failed",
			err,
		))
	}

	return table.DoExpectation(treatment, level, controls...)
}

/*
AbductiveCounterfactual performs abduction, intervention, then prediction,
preserving the precision the Table derives from its reconstruction error rather
than recomputing a second, differently-scaled one here.
*/
func (engine DefaultCausalEngine) AbductiveCounterfactual(
	rows [][]float64,
	target int,
	minimumRows int,
	features []int,
	linear bool,
	actual []float64,
	treatment int,
	level float64,
) (float64, float64, float64, error) {
	table, err := getOrNewTable(rows, target, minimumRows, linear)

	if err != nil {
		return 0, 0, 0, errnie.Error(errnie.Err(
			errnie.PreconditionFailed,
			"[mcts] causal table construction failed",
			err,
		))
	}

	return table.AbductiveCounterfactual(features, actual, treatment, level)
}
