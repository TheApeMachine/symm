package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Erfc is the complementary error function: the Gaussian tail mass beyond a
value. It converts a standardised deviation into how surprising that
deviation is, which is what turns a measurement into evidence.
*/
type Erfc struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewErfc configures the register's starting value.
*/
func NewErfc(state core.Primitive) *Erfc {
	return &Erfc{
		current: state,
	}
}

/*
Next transforms the incoming value and holds the result. Offered
nothing, Yield answers with the register untouched, so a composition can
begin here.
*/
func (erfc *Erfc) Next(in core.Primitive) core.Primitive {
	erfc.current = core.Yield(
		erfc.current, in, func(held, value float64) float64 {
			return math.Erfc(value)
		},
	)

	return erfc.current
}

/*
Read surfaces the register for the boundary.
*/
func (erfc *Erfc) Read() any {
	return erfc.current.Read()
}
