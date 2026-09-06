package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sign is the direction transfer, discarding magnitude. It is the complement of
Absolute: where that answers how far, this answers which way, and composing
the two recovers the original value.
*/
type Sign struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewSign configures the register's starting value.
*/
func NewSign(state core.Primitive) *Sign {
	return &Sign{
		current: state,
	}
}

/*
Next transforms the incoming value and holds the result. Offered
nothing, Yield answers with the register untouched, so a composition can
begin here.
*/
func (sign *Sign) Next(in core.Primitive) core.Primitive {
	sign.current = core.Yield(
		sign.current, in, func(held, value float64) float64 {
			if value > 0 {
				return 1
			}

			if value < 0 {
				return -1
			}

			return 0
		},
	)

	return sign.current
}

/*
Read surfaces the register for the boundary.
*/
func (sign *Sign) Read() any {
	return sign.current.Read()
}
