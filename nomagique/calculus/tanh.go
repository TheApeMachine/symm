package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Tanh maps the whole real line into the open interval between minus one and
one. It is how a composition bounds an unbounded quantity without clamping
it, since values keep their order however far out they came from.
*/
type Tanh struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewTanh configures the register's starting value.
*/
func NewTanh(state core.Primitive) *Tanh {
	return &Tanh{
		current: state,
	}
}

/*
Next transforms the incoming value and holds the result. Offered
nothing, Yield answers with the register untouched, so a composition can
begin here.
*/
func (tanh *Tanh) Next(in core.Primitive) core.Primitive {
	tanh.current = core.Yield(
		tanh.current, in, func(held, value float64) float64 {
			return math.Tanh(value)
		},
	)

	return tanh.current
}

/*
Read surfaces the register for the boundary.
*/
func (tanh *Tanh) Read() any {
	return tanh.current.Read()
}
