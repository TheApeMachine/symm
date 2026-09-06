package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Add is the first of the two operations that make arithmetic a field, and it
accumulates: the constructor configures a register, and every step folds
whatever the incoming Primitive yields into it. A single Add stepped
repeatedly is therefore a running sum, which is why the system needs no
separate summing Primitive.

Subtract is its inverse, and zero is its identity.
*/
type Add struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewAdd configures the register's starting value.
*/
func NewAdd(state core.Primitive) *Add {
	return &Add{
		current: state,
	}
}

/*
Next folds everything the incoming Primitive yields into the register and
hands back the result. Offered nothing, Yield answers with the register
untouched, so a composition can begin here.
*/
func (add *Add) Next(in core.Primitive) core.Primitive {
	add.current = core.Yield(
		add.current, in, func(held, value float64) float64 {
			return held + value
		},
	)

	return add.current
}

/*
Read surfaces the register for the boundary.
*/
func (add *Add) Read() any {
	return add.current.Read()
}
