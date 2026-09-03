package nomagique

import (
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

// Number is the top-level composition builder.
// It accepts any composition of Nodes and returns a runnable Pipeline.
func Number(root Node) *Pipeline {
	return &Pipeline{root: root}
}
