package probability

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Argmax preserves a winning value's ordinal and value through comparison.
*/
type Argmax struct {
	*core.PrimitiveError
	bestValue    float64
	bestIndex    float64
	currentIndex float64
	seen         bool
}

func NewArgmax() core.Primitive {
	return &Argmax{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Argmax) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)

			if !op.seen || val > op.bestValue {
				op.bestValue = val
				op.bestIndex = op.currentIndex
				op.seen = true
			}

			op.currentIndex++
		}

		if !op.seen {
			return
		}

		for value := range data.NewValue(op.bestIndex, op.bestValue).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
