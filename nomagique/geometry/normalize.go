package geometry

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Normalize scales coordinate pairs to unit energy.
*/
type Normalize struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewNormalize() *Normalize {
	output := data.NewOutputMap()
	output.Values["x"] = 0
	output.Values["y"] = 0
	output.Values["norm"] = 0

	return &Normalize{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"x", "x",
			"y", "y",
		),
		output: output,
	}
}

func (op *Normalize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			coordinateX, xOK := values.Values["x"]
			coordinateY, yOK := values.Values["y"]

			if !xOK || !yOK {
				op.Error(core.ErrNotHeld)
				return
			}

			norm := math.Hypot(coordinateX, coordinateY)
			normalizedX := coordinateX
			normalizedY := coordinateY

			if norm > 0 {
				normalizedX = coordinateX / norm
				normalizedY = coordinateY / norm
			}

			op.output.Values["x"] = normalizedX
			op.output.Values["y"] = normalizedY
			op.output.Values["norm"] = norm

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
