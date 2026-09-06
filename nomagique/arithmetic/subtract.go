package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Subtract is the additive inverse: it undoes what Add does, and the two
together give the field its additive structure. Like every arithmetic
Primitive it accumulates, so a single Subtract stepped repeatedly is a
running drawdown.

There is no floor. A register that passes zero keeps going negative, because
clamping would be a policy decision, and policy belongs to whatever composes
these, not to the operation itself.
*/
type Subtract struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewSubtract configures the register's starting value.
*/
func NewSubtract(state core.Primitive) *Subtract {
	return &Subtract{
		current: state,
	}
}

/*
Next folds everything the incoming Primitive yields into the register and
hands back the result. Offered nothing, Yield answers with the register
untouched, so a composition can begin here.
*/
func (subtract *Subtract) Next(in core.Primitive) core.Primitive {
	subtract.current = core.Yield(subtract.current, in, func(held, value float64) float64 {
		return held - value
	})

	return subtract.current
}

/*
Read surfaces the register for the boundary.
*/
func (subtract *Subtract) Read() any {
	return subtract.current.Read()
}
