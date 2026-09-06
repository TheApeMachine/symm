package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Curvature is how sharply a profile falls away from its peak, normalized by
the coordinate it was sampled on.

Prominence says how far the peak stands out at the resolution it happened to
be searched at; curvature says the same thing in the units the coordinate is
measured in, so two searches run at different resolutions can be compared.

A peak at the edge of the range, or a profile sampled at no spacing at all,
leaves the value held standing rather than reporting a shape nothing was
measured for.
*/
type Curvature struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewCurvature configures the value held before anything has been shown.
*/
func NewCurvature(state core.Primitive) *Curvature {
	return &Curvature{
		current: state,
	}
}

/*
Next gathers the incoming profile and holds the second difference across its
peak, per squared unit of the coordinate.
*/
func (curvature *Curvature) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]Point(nil)), in,
		func(held, arriving []Point) []Point {
			return append(held, arriving...)
		},
	)

	profile := core.To[[]Point](gathered)
	index := peakIndex(profile)

	if index <= 0 || index+1 >= len(profile) {
		curvature.current.Error(gathered.Error())

		return curvature.current
	}

	spacing := (profile[index+1].X - profile[index-1].X) / 2

	if spacing == 0 {
		curvature.current.Error(gathered.Error())

		return curvature.current
	}

	second := 2*math.Abs(profile[index].Y) -
		math.Abs(profile[index-1].Y) - math.Abs(profile[index+1].Y)

	curvature.current = core.From(second / (spacing * spacing))
	curvature.current.Error(gathered.Error())

	return curvature.current
}

/*
Read surfaces the curvature for the boundary.
*/
func (curvature *Curvature) Read() any {
	return curvature.current.Read()
}
