package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Maximum holds the largest value it has been shown, and is the mirror of
Minimum. Together they bound what a composition has observed without
retaining any of it.
*/
type Maximum struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewMaximum configures the register's starting value.
*/
func NewMaximum(state core.Primitive) *Maximum {
	return &Maximum{
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
func (maximum *Maximum) Next(in core.Primitive) core.Primitive {
	maximum.current = core.Yield(
		maximum.current, in, func(held, value float64) float64 {
			return math.Max(held, value)
		},
	)
	return maximum.current
}

/*
Read surfaces the register for the boundary.
*/
func (maximum *Maximum) Read() any {
	return maximum.current.Read()
}
