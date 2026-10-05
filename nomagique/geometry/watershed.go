package geometry

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Watershed finds density peaks and assigns basin membership.
*/
type Watershed struct {
	*core.PrimitiveError
	maxAuthority float64
	peakX        float64
	peakY        float64
	seen         bool
	input        data.Map[string]
	output       data.Map[float64]
}

func NewWatershed() *Watershed {
	output := data.NewOutputMap()
	output.Values["basin"] = 0
	output.Values["peak"] = 0

	return &Watershed{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"x", "x",
			"y", "y",
			"authority", "authority",
		),
		output: output,
	}
}

func (op *Watershed) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			coordX, xOK := values.Values["x"]
			coordY, yOK := values.Values["y"]
			authority, authOK := values.Values["authority"]

			if !xOK || !yOK || !authOK {
				op.Error(core.ErrNotHeld)
				return
			}

			peak := 0.0

			if !op.seen || authority > op.maxAuthority {
				op.maxAuthority = authority
				op.peakX = coordX
				op.peakY = coordY
				op.seen = true
				peak = 1.0
			}

			quadrant := 0.0

			if coordX >= 0.5 {
				quadrant += 1.0
			}

			if coordY >= 0.5 {
				quadrant += 2.0
			}

			op.output.Values["basin"] = quadrant
			op.output.Values["peak"] = peak

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
