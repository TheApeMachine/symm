package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Square is the second power. It is how a composition weighs large deviations
far more heavily than small ones, which is what makes it the basis of every
variance and every least-squares fit in the system.
*/
type Square struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewSquare configures the register's starting value.
*/
func NewSquare(state core.Primitive) *Square {
	return &Square{
		current: state,
	}
}

/*
Next transforms the incoming value and holds the result. Offered
nothing, Yield answers with the register untouched, so a composition can
begin here.
*/
func (square *Square) Next(in core.Primitive) core.Primitive {
	square.current = core.Yield(
		square.current, in, func(held, value float64) float64 {
			return value * value
		},
	)

	return square.current
}

/*
Read surfaces the register for the boundary.
*/
func (square *Square) Read() any {
	return square.current.Read()
}
