package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Prominence is how far a profile's peak stands above its immediate
neighbours: how isolated the maximum is, rather than how high.

A broad rise says the two paths are related over a range of offsets and the
exact peak is arbitrary; a spike says one offset is doing the work. Height
alone cannot tell those apart, which is why the number a search reports is
never enough on its own.

A peak at the edge of the searched range has a neighbour missing, so the
value held stands: an edge peak is a statement that the range was too narrow,
not a prominence of zero.
*/
type Prominence struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewProminence configures the value held before anything has been shown.
*/
func NewProminence(state core.Primitive) *Prominence {
	return &Prominence{
		current: state,
	}
}

/*
Next gathers the incoming profile and holds how far its peak stands out.
*/
func (prominence *Prominence) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]Point(nil)), in,
		func(held, arriving []Point) []Point {
			return append(held, arriving...)
		},
	)

	profile := core.To[[]Point](gathered)
	index := peakIndex(profile)

	if index <= 0 || index+1 >= len(profile) {
		prominence.current.Error(gathered.Error())

		return prominence.current
	}

	shoulders := (math.Abs(profile[index-1].Y) + math.Abs(profile[index+1].Y)) / 2

	prominence.current = core.From(math.Abs(profile[index].Y) - shoulders)
	prominence.current.Error(gathered.Error())

	return prominence.current
}

/*
Read surfaces the prominence for the boundary.
*/
func (prominence *Prominence) Read() any {
	return prominence.current.Read()
}
