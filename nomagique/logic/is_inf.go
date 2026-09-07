package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

// IsInf tests whether one number is infinite, irrespective of sign.
type IsInf struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewIsInf() *IsInf {
	return &IsInf{seed: transport.NewIO(core.From(false))}
}
func (p *IsInf) Next(in core.Primitive) core.Primitive {
	result := core.Yield(p.seed, in, func(_ bool, v float64) bool { return math.IsInf(v, 0) }, p)
	if result != nil {
		p.current = result
	}
	return result
}
func (p *IsInf) Read() any { return core.To[any](p.current) }
