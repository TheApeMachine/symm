package nomagique

import (
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/types"
)

// Re-export core topological algebra and types
type (
	Scalar    = types.Scalar
	Node      = types.Node
	Chain     = types.Chain
	Split     = types.Split
	Identity  = types.Identity
	Reduction = types.Reduction
	Router    = types.Router
	Tap       = types.Tap
	Probe     = types.Probe
)

// Pipeline wraps a composed Node graph, exposing steppable evaluation methods.
type Pipeline struct {
	root Node

	// tick counts observations, so a stateful node reached by several paths
	// of the graph advances once per observation rather than once per path.
	//
	// It is a pointer because the guards inside the composition hold this
	// exact counter: copying a Pipeline by value would leave them pointing at
	// the counter of a discarded original, and every guarded node would then
	// see an observation that never advances.
	tick *types.Tick
}

/*
Step advances the composition with one observation. Opening the observation
first is what lets a stateful node distinguish being reached again in the same
observation from being reached in the next one.
*/
func (p *Pipeline) Step(x Scalar) Scalar {
	p.tick.Advance()

	return p.root.Step(x)
}

func (p *Pipeline) Apply(x Scalar) Scalar {
	return p.root.Step(x)
}

func (p *Pipeline) Root() Node {
	return p.root
}

/*
Number is the top-level composition builder.

It accepts any composition of Nodes and returns a runnable Pipeline. If the
composition terminates in a data.Projection, the builder binds that projection
to the graph it terminates, so the projection harvests its upstream stages
without the caller wiring them by hand. This is what lets a whole measurement
be one nested literal with no intermediate variables.
*/
func Number(root Node) *Pipeline {
	pipeline := &Pipeline{root: root, tick: &types.Tick{}}

	types.Bind(root, pipeline.tick)

	return pipeline
}

/*
Measurement returns the Measurement this pipeline's terminal projection
published on its most recent Step, or nil when the composition has no
projection to publish one.
*/
func (p *Pipeline) Measurement() *data.Measurement[float64] {
	if measurer, ok := p.root.(interface {
		Measurement() *data.Measurement[float64]
	}); ok {
		return measurer.Measurement()
	}

	var measurement *data.Measurement[float64]

	types.Walk(p.root, func(node Node) {
		if projection, ok := node.(interface {
			Measurement() *data.Measurement[float64]
		}); ok {
			measurement = projection.Measurement()
		}
	})

	return measurement
}
