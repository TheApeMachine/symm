package transport

import "github.com/theapemachine/symm/nomagique/core"

// Discard consumes a run without yielding values. Failures are retained.
type Discard struct{ core.PrimitiveError }

func NewDiscard() *Discard { return &Discard{} }
func (discard *Discard) Next(in core.Primitive) core.Primitive {
	core.Yield(NewIO(core.From(0)), in, func(held int, _ core.Primitive) int { return held }, discard)
	return nil
}
func (discard *Discard) Read() any { return nil }
