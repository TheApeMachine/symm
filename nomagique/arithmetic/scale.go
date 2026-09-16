package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ScaleInput is a vector and the scalar that multiplies every member.
*/
type ScaleInput struct {
	Values []float64
	Factor float64
}

/*
Scale multiplies each member by one scalar.
*/
type Scale struct {
	*core.PrimitiveError

	out []float64
}

func NewScale() *Scale {
	return &Scale{PrimitiveError: core.NewPrimitiveError()}
}

func (scale *Scale) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ScaleInput)(arriving)

			if len(scale.out) != len(input.Values) {
				scale.out = make([]float64, len(input.Values))
			}

			for index, value := range input.Values {
				scale.out[index] = value * input.Factor
			}

			if !yield(unsafe.Pointer(&scale.out)) {
				return
			}
		}
	}
}
