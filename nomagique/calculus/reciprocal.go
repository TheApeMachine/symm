package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Reciprocal is the multiplicative inverse. It is the operation that turns a
quantity into the thing that cancels it under multiplication, so a
composition can divide by multiplying.
*/
type Reciprocal struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewReciprocal configures the register's starting value.
*/
func NewReciprocal(state core.Primitive) *Reciprocal {
	return &Reciprocal{
		current: state,
	}
}

/*
Next transforms the incoming value and holds the result. Offered nothing,
Yield answers with the register untouched, so a composition can begin here.

Zero has no inverse, and the register stands rather than taking an infinity.
*/
func (reciprocal *Reciprocal) Next(in core.Primitive) core.Primitive {
	reciprocal.current = core.Yield(
		reciprocal.current, in, func(held, value float64) float64 {
			if value == 0 {
				return held
			}

			return 1 / value
		},
	)

	return reciprocal.current
}

/*
Read surfaces the register for the boundary.
*/
func (reciprocal *Reciprocal) Read() any {
	return reciprocal.current.Read()
}
