package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Atanh is the inverse of Tanh, mapping the open interval between minus one
and one back onto the whole real line. It is how a bounded quantity such as
a correlation is moved somewhere its dispersion stops depending on where it
sat, which is what makes it comparable.
*/
type Atanh struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewAtanh configures the register's starting value.
*/
func NewAtanh(state core.Primitive) *Atanh {
	return &Atanh{
		current: state,
	}
}

/*
Next transforms the incoming value and holds the result. Offered nothing,
Yield answers with the register untouched, so a composition can begin here.

The transform is undefined at and beyond the boundary, where a saturated
value carries no information about how saturated it is, and the register
stands rather than taking an infinity.
*/
func (atanh *Atanh) Next(in core.Primitive) core.Primitive {
	atanh.current = core.Yield(
		atanh.current, in, func(held, value float64) float64 {
			if value <= -1 || value >= 1 {
				return held
			}

			return math.Atanh(value)
		},
	)

	return atanh.current
}

/*
Read surfaces the register for the boundary.
*/
func (atanh *Atanh) Read() any {
	return atanh.current.Read()
}
