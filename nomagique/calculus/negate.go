package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Negate is the additive inverse. It is the operation that turns a quantity
into the thing that cancels it, so a composition can subtract by adding.
*/
type Negate struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewNegate configures the register's starting value.
*/
func NewNegate(state core.Primitive) *Negate {
	return &Negate{
		current: state,
	}
}

/*
Next transforms the incoming value and holds the result. Offered
nothing, Yield answers with the register untouched, so a composition can
begin here.
*/
func (negate *Negate) Next(in core.Primitive) core.Primitive {
	negate.current = core.Yield(
		negate.current, in, func(held, value float64) float64 {
			return -value
		},
	)

	return negate.current
}

/*
Read surfaces the register for the boundary.
*/
func (negate *Negate) Read() any {
	return negate.current.Read()
}
