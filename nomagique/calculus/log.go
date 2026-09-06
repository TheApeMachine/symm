package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Log is the natural logarithm, and the inverse of Exp. It turns a
multiplicative quantity into an additive one, so a composition can add
where the underlying process compounds.
*/
type Log struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewLog configures the register's starting value.
*/
func NewLog(state core.Primitive) *Log {
	return &Log{
		current: state,
	}
}

/*
Next transforms the incoming value and holds the result. Offered nothing,
Yield answers with the register untouched, so a composition can begin here.

The logarithm is undefined at and below zero, and the register stands
rather than taking an infinity or a NaN.
*/
func (log *Log) Next(in core.Primitive) core.Primitive {
	log.current = core.Yield(
		log.current, in, func(held, value float64) float64 {
			if value <= 0 {
				return held
			}

			return math.Log(value)
		},
	)

	return log.current
}

/*
Read surfaces the register for the boundary.
*/
func (log *Log) Read() any {
	return log.current.Read()
}
