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
Yields angle and phase.
*/
type PhasePath struct {
	*core.PrimitiveError
	index float64
}

func NewPhasePath() *PhasePath {
	return &PhasePath{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *PhasePath) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			samples := *(*float64)(arriving)

			if samples <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			angle := 2 * math.Pi * math.Mod(op.index, samples) / samples
			op.index++
			phase := angle / (2 * math.Pi)

			for value := range data.NewValue(angle, phase).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
