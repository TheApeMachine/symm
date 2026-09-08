package cognition

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
CognitiveResult adapts Engine.Evaluate into nomagique's streaming algebra while
retaining the immutable context that produced the reading. It is a boundary
value, not a second evaluation owner: Engine remains the single evaluator.
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
	core.PrimitiveError
	engine  *Engine
	seed    *transport.IO
	current core.Primitive
}

func NewPrimitive(engine *Engine) *Primitive {
	if engine == nil {
		engine = NewEngine(DefaultConfig())
	}

	return &Primitive{
		engine: engine,
		seed:   transport.NewIO(core.From(CognitiveResult{})),
	}
}

func (p *Primitive) Next(input core.Primitive) core.Primitive {
	result := core.Yield(p.seed, input, func(_ CognitiveResult, fields map[string]core.Primitive) CognitiveResult {
		contextBytes, err := core.Field[[]byte](fields, "context")

		if err != nil {
			p.Error(err)
			return CognitiveResult{}
		}

		// If a training label is provided, learn first
		classBytes, err := core.Field[[]byte](fields, "class")
		if err == nil && len(classBytes) > 0 {
			p.engine.Observe(contextBytes, classBytes)
		}

		return CognitiveResult{
			Evaluation: p.engine.Evaluate(contextBytes),
			Context:    contextBytes,
			Class:      classBytes,
		}
	}, p)

	if result != nil {
		p.current = result
	}

	return result
}

func (p *Primitive) Read() any {
	return core.To[any](p.current)
}
