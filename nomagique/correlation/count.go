package correlation

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Count is how many values a run holds.

Support is the quantity every estimate here is finally judged on — a
correlation over two coincidences is arithmetic, not evidence — and support
is a count of what was actually paired. Counting is not summing, so it is not
Add's business, and it is not the pairing's business either.
*/
type Count struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewCount configures the value held before anything has been shown.
*/
func NewCount(state core.Primitive) *Count {
	return &Count{
		current: state,
	}
}

/*
Next gathers the incoming run and holds how many values it carried.
*/
func (count *Count) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]float64(nil)), in,
		func(held, arriving []float64) []float64 {
			return append(held, arriving...)
		},
	)

	count.current = core.From(float64(len(core.To[[]float64](gathered))))
	count.current.Error(gathered.Error())

	return count.current
}

/*
Read surfaces the count for the boundary.
*/
func (count *Count) Read() any {
	return count.current.Read()
}
