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
}

func (p *Pipeline) Step(x Scalar) Scalar {
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
	types.Bind(root)

	return &Pipeline{root: root}
}

/*
Measurement returns the Measurement this pipeline's terminal projection
published on its most recent Step, or nil when the composition has no
projection to publish one.
*/
func (p *Pipeline) Measurement() *data.Measurement[float64] {
	var measurement *data.Measurement[float64]

	types.Walk(p.root, func(node Node) {
		if projection, ok := node.(*data.Projection); ok {
			measurement = projection.Measurement()
		}
	})

	return measurement
}
