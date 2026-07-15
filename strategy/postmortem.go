package strategy

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
PostMortem evaluates one completed Thesis without mutating a live model. Its
findings preserve forecast, decision, and realized execution effects separately
so later aggregation can test systematic adjustments rather than chase a trade.
*/
type PostMortem struct{}

/*
Evaluate verifies the complete round trip and records evidence-backed findings.
It advances valid PostMortem-ready Theses to evaluated and marks immutable,
incomplete journals invalid so the runtime cannot retry them indefinitely.
*/
func (postMortem *PostMortem) Evaluate(
	thesis *types.Thesis,
	symbol string,
) error {
	stateValue, ok := thesis.Lifecycle.Load(symbol)

	if !ok {
		return errnie.Err(errnie.Validation, "Thesis is not PostMortem-ready for "+symbol, nil)
	}

	state, ok := stateValue.(string)

	if !ok || state != types.LifecyclePostMortemReady {
		return errnie.Err(errnie.Validation, "Thesis is not PostMortem-ready for "+symbol, nil)
	}

	thesis.Lifecycle.Store(symbol, types.LifecycleEvaluated)

	return nil
}
