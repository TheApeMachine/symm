package learning

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/* modelPrior owns a context's retained state and unresolved tickets, never duplicate moments. */
type modelPrior struct {
	evaluator *priorEvaluator
	state     core.Primitive
	pending   uint64
}

func (prior *modelPrior) reading(epoch uint64) PriorReading {
	fields, err := prior.evaluator.evaluate(prior, core.Record(map[string]any{"epoch": epoch}))
	if err != nil {
		panic(err)
	}
	reading, err := ProjectPrior(fields)
	if err != nil {
		panic(err)
	}
	reading.Pending = prior.pending
	return reading
}
