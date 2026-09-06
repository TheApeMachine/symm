package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Absolute is the magnitude transfer, discarding direction. It answers how
far a value sits from zero without saying which way, which is what a
composition asks for whenever the size of a deviation matters and its sign
does not.
*/
type Absolute struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewAbsolute configures the register's starting value.
*/
func NewAbsolute(state core.Primitive) *Absolute {
	return &Absolute{
		current: state,
	}
}

/*
Next transforms the incoming value and holds the result. Offered
nothing, Yield answers with the register untouched, so a composition can
begin here.
*/
func (absolute *Absolute) Next(in core.Primitive) core.Primitive {
	absolute.current = core.Yield(
		absolute.current, in, func(held, value float64) float64 {
			return math.Abs(value)
		},
	)

	return absolute.current
}

/*
Read surfaces the register for the boundary.
*/
func (absolute *Absolute) Read() any {
	return absolute.current.Read()
}
