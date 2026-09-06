package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sqrt is the principal square root, and the inverse of Square. It brings a
squared quantity back to the units it was measured in, which is what turns a
variance into a dispersion.
*/
type Sqrt struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewSqrt configures the register's starting value.
*/
func NewSqrt(state core.Primitive) *Sqrt {
	return &Sqrt{
		current: state,
	}
}

/*
Next transforms the incoming value and holds the result. Offered nothing,
Yield answers with the register untouched, so a composition can begin here.

A negative value has no real root, and the register stands rather than
taking a NaN.
*/
func (sqrt *Sqrt) Next(in core.Primitive) core.Primitive {
	sqrt.current = core.Yield(
		sqrt.current, in, func(held, value float64) float64 {
			if value < 0 {
				return held
			}

			return math.Sqrt(value)
		},
	)

	return sqrt.current
}

/*
Read surfaces the register for the boundary.
*/
func (sqrt *Sqrt) Read() any {
	return sqrt.current.Read()
}
