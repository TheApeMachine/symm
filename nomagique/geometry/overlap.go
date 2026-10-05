package geometry

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Overlap owns the normalized inner product and distance between coordinate pairs.
*/
type Overlap struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewOverlap() *Overlap {
	output := data.NewOutputMap()
	output.Values["overlap"] = 0
	output.Values["affinity"] = 0
	output.Values["distance"] = 0

	return &Overlap{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"x1", "x1",
			"y1", "y1",
			"x2", "x2",
			"y2", "y2",
		),
		output: output,
	}
}

func (op *Overlap) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			firstX, fxOK := values.Values["x1"]
			firstY, fyOK := values.Values["y1"]
			secondX, sxOK := values.Values["x2"]
			secondY, syOK := values.Values["y2"]

			if !fxOK || !fyOK || !sxOK || !syOK {
				op.Error(core.ErrNotHeld)
				return
			}

			dot := firstX*secondX + firstY*secondY
			firstNorm := math.Hypot(firstX, firstY)
			secondNorm := math.Hypot(secondX, secondY)
			affinity := 0.0

			if firstNorm > 0 && secondNorm > 0 {
				affinity = dot / (firstNorm * secondNorm)
			}

			distance := math.Hypot(secondX-firstX, secondY-firstY)

			op.output.Values["overlap"] = dot
			op.output.Values["affinity"] = affinity
			op.output.Values["distance"] = distance

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
