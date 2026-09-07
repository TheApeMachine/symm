package learning

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
priorEvaluator owns one reusable transition graph for a serialized Model.
Each modelPrior retains its own immutable state record; only execution scratch
is shared. Switching contexts never shares their moments or causal clocks.
*/
type priorEvaluator struct {
	memory *store.Retained
	graph  core.Primitive
}

func newPriorEvaluator(memory float64) *priorEvaluator {
	evaluator := &priorEvaluator{memory: NewPriorMemory()}
	evaluator.graph = NewPrior(store.NewConstant(core.From(memory)), evaluator.memory)
	return evaluator
}

func (evaluator *priorEvaluator) evaluate(prior *modelPrior, input core.Primitive) (map[string]core.Primitive, error) {
	if _, err := transport.Evaluate[map[string]core.Primitive](evaluator.memory, prior.state); err != nil {
		return nil, err
	}

	fields, err := transport.Evaluate[map[string]core.Primitive](evaluator.graph, input)

	if err != nil {
		return nil, err
	}

	prior.state = core.From(core.To[map[string]core.Primitive](evaluator.memory))
	return fields, nil
}
