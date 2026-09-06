package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Convert is an explicit numerical representation change. Timestamp differences
// are taken in int64 before conversion, avoiding subtraction of float epochs.
type Convert[A, B core.Numeric] struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewConvert[A, B core.Numeric]() *Convert[A, B] {
	var zero B
	return &Convert[A, B]{current: store.NewRetained(nil), seed: transport.NewIO(core.From(zero))}
}
func (convert *Convert[A, B]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(convert.seed, in, func(_ B, v A) B { return B(v) }, convert)
	transport.NewDiscard().Next(transport.NewApply(convert.current, transport.NewIO(result)))
	return result
}
func (convert *Convert[A, B]) Read() any { return core.To[any](convert.current) }
