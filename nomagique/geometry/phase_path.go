package geometry

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
PhasePath owns angular path construction over the full circle.
*/
type PhasePath struct {
	*core.PrimitiveError
	index  float64
	input  data.Map[string]
	output data.Map[float64]
}

func NewPhasePath() *PhasePath {
	output := data.NewOutputMap()
	output.Values["angle"] = 0
	output.Values["phase"] = 0

	return &PhasePath{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("samples", "samples"),
		output:         output,
	}
}

func (op *PhasePath) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			samples, ok := values.Values["samples"]

			if !ok {
				op.Error(core.ErrNotHeld)
				return
			}

			if samples <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			angle := 2 * math.Pi * math.Mod(op.index, samples) / samples
			op.index++

			op.output.Values["angle"] = angle
			op.output.Values["phase"] = angle / (2 * math.Pi)

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
