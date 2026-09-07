package equation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Peak owns one delivery's maximum absolute ordinate and its original point. */
type Peak struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

/* peakSelection keeps fixed typed selection state rather than nested pair records. */
type peakSelection struct {
	point     core.Primitive
	index     int
	magnitude float64
}

/* NewPeak selects the first absolute maximum; an undefined ordinate remains explicit. */
func NewPeak() core.Primitive {
	return transport.NewPipe(
		&Peak{seed: transport.NewIO(core.From(peakSelection{}))},
		transport.NewSpread[core.Primitive](),
	)
}

func (peak *Peak) Next(input core.Primitive) core.Primitive {
	index := 0
	result := core.Yield(peak.seed, input,
		func(held peakSelection, point core.Primitive) peakSelection {
			fields := core.To[map[string]core.Primitive](point)
			ordinate, err := core.Field[float64](fields, "y")
			peak.Error(point.Error(), err)
			magnitude := math.Abs(ordinate)

			if held.point == nil || math.IsNaN(ordinate) || magnitude > held.magnitude {
				held = peakSelection{point: point, index: index, magnitude: magnitude}
			}
			index++
			return held
		}, peak)

	if result == nil {
		return nil
	}
	selection := core.To[peakSelection](result)
	points := []core.Primitive{}

	if selection.point != nil {
		points = append(points, core.Record(map[string]any{"index": float64(selection.index), "point": selection.point}))
	}
	peak.current = core.From(points)
	peak.current.Error(result.Error())
	return peak.current
}

func (peak *Peak) Read() any { return core.To[any](peak.current) }
