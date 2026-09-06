package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Exp is the natural exponential. It turns an additive quantity into a
multiplicative one, which is what a composition needs whenever a rate
compounds rather than accumulates.
*/
type Exp struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewExp configures the register's starting value.
*/
func NewExp(state core.Primitive) *Exp {
	return &Exp{
		current: state,
	}
}

/*
Next transforms the incoming value and holds the result. Offered
nothing, Yield answers with the register untouched, so a composition can
begin here.
*/
func (exp *Exp) Next(in core.Primitive) core.Primitive {
	exp.current = core.Yield(
		exp.current, in, func(held, value float64) float64 {
			return math.Exp(value)
		},
	)

	return exp.current
}

/*
Read surfaces the register for the boundary.
*/
func (exp *Exp) Read() any {
	return exp.current.Read()
}
