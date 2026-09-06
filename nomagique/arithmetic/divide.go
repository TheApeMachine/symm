package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Divide is the multiplicative inverse, and it carries the field's one genuine
hole: zero has no inverse, so there is nothing correct for this operation to
return when it meets one.

The register stands unchanged in that case, which is a choice and not a law.
It keeps a composition alive at the cost of silently skipping a step, and the
alternative is to let the division happen and send an infinity downstream
where it will surface loudly. Anything composing a Divide over data that can
legitimately reach zero should be aware it is relying on this behaviour.
*/
type Divide struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewDivide configures the register's starting value.
*/
func NewDivide(state core.Primitive) *Divide {
	return &Divide{
		current: state,
	}
}

/*
Next folds everything the incoming Primitive yields into the register and
hands back the result. Offered nothing, Yield answers with the register
untouched, so a composition can begin here.
*/
func (divide *Divide) Next(in core.Primitive) core.Primitive {
	divide.current = core.Yield(divide.current, in, func(held, value float64) float64 {
		if value == 0 {
			return held
		}

		return held / value
	})

	return divide.current
}

/*
Read surfaces the register for the boundary.
*/
func (divide *Divide) Read() any {
	return divide.current.Read()
}
