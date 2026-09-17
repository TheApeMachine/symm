package geometry

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Weight owns the strength and direction of a relational connection.
When stepped without input, it yields its strength then direction as scalar pointers.
When stepped with inputs, it attenuates/scales each arriving scalar by strength.
*/
type Weight struct {
	*core.PrimitiveError
	Strength  float64
	Direction float64
	scaled    float64
}

func NewWeight(strength, direction float64) *Weight {
	return &Weight{
		PrimitiveError: core.NewPrimitiveError(),
		Strength:       strength,
		Direction:      direction,
	}
}

func (weight *Weight) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			if !yield(unsafe.Pointer(&weight.Strength)) {
				return
			}

			yield(unsafe.Pointer(&weight.Direction))
			return
		}

		for arriving := range in {
			val := *(*float64)(arriving)
			weight.scaled = val * weight.Strength

			if !yield(unsafe.Pointer(&weight.scaled)) {
				return
			}
		}
	}
}
