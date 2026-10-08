package causal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Backdoor standardizes predictions over observed evidence after replacing the
treatment coordinate with the interventional level.
*/
type Backdoor struct {
	*core.PrimitiveError
	tolerance float64
}

func NewBackdoor(tolerance float64) core.Primitive {
	return &Backdoor{
		PrimitiveError: core.NewPrimitiveError(),
		tolerance:      tolerance,
	}
}

func (op *Backdoor) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [3]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 3 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 3 {
			op.Error(core.ErrShape)
			return
		}

		level := values[0]
		baseline := values[1]
		effect := values[2]

		expectation := baseline + effect*level
		defined := 1.0

		for value := range data.NewValue(expectation, defined).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
