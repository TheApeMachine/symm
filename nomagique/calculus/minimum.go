package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Minimum holds the smallest value it has been shown. Where a reduction over a
slice needs the whole slice in hand, this keeps the running extreme instead,
so a composition never has to retain what it has already seen.
*/
type Minimum struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewMinimum configures the register's starting value.
*/
func NewMinimum(state core.Primitive) *Minimum {
	return &Minimum{
		current: state,
	}
}

/*
Next folds the incoming value into the register and holds the result. Offered
nothing, Yield answers with the register untouched, so a composition can
begin here.

A comparison against a NaN is false whichever way it is written, so comparing
directly would leave the register standing and swallow the reading. The
extreme is taken through math, which propagates it instead.
*/
func (minimum *Minimum) Next(in core.Primitive) core.Primitive {
	minimum.current = core.Yield(
		minimum.current, in, func(held, value float64) float64 {
			return math.Min(held, value)
		},
	)
	return minimum.current
}

/*
Read surfaces the register for the boundary.
*/
func (minimum *Minimum) Read() any {
	return minimum.current.Read()
}
