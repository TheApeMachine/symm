package cognition

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Input is one context and an optional training class.
*/
type Input struct {
	Context []byte
	Class   []byte
}

/*
CognitiveResult adapts Engine.Evaluate into nomagique's streaming algebra while
retaining the immutable context that produced the reading. Engine remains the
single evaluator.
*/
type CognitiveResult struct {
	Evaluation
	Context []byte
	Class   []byte
}

/*
Primitive adapts the Engine into nomagique's streaming algebra.
*/
type Primitive struct {
	core.Base[Input, CognitiveResult]
	engine *Engine
}

func NewPrimitive(engine *Engine) *Primitive {
	if engine == nil {
		engine = NewEngine(DefaultConfig())
	}

	return &Primitive{engine: engine}
}

func (op *Primitive) Next(
	in iter.Seq[core.Primitive[Input, Input]],
) iter.Seq[core.Primitive[CognitiveResult, CognitiveResult]] {
	return func(yield func(core.Primitive[CognitiveResult, CognitiveResult]) bool) {
		for arriving := range in {
			input := arriving.Read()

			if len(input.Class) > 0 {
				op.engine.Observe(input.Context, input.Class)
			}

			if !yield(op.Carrier(CognitiveResult{
				Evaluation: op.engine.Evaluate(input.Context),
				Context:    input.Context,
				Class:      input.Class,
			})) {
				return
			}
		}
	}
}
