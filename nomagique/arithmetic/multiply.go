package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Multiply is the second of the two operations that make arithmetic a field,
and like the rest it accumulates. Divide is its inverse, and one is its
identity.

Because it compounds, a Multiply fed a constant factor below one is an
exponential decay, and fed a factor above one is exponential growth. This is
why the system has no decay Primitive: decay is not a concept of its own, it
is this operation with a particular input, and adding a type for it would be
the kind of special case that makes an algebra collapse into a library.

Zero is absorbing. A register that meets it cannot recover, which is correct:
the operation has no memory of what it held, and inventing one would make
Multiply something other than multiplication.
*/
type Multiply struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewMultiply configures the register's starting value.
*/
func NewMultiply(state core.Primitive) *Multiply {
	return &Multiply{
		current: state,
	}
}

/*
Next folds everything the incoming Primitive yields into the register and
hands back the result. Offered nothing, Yield answers with the register
untouched, so a composition can begin here.
*/
func (multiply *Multiply) Next(in core.Primitive) core.Primitive {
	multiply.current = core.Yield(multiply.current, in, func(held, value float64) float64 {
		return held * value
	})

	return multiply.current
}

/*
Read surfaces the register for the boundary.
*/
func (multiply *Multiply) Read() any {
	return multiply.current.Read()
}
