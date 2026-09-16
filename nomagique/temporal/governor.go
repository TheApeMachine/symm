package temporal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Governor retains a tail of arrivals whose length is the configured capacity
and hands that tail, as one collection, to a reduction. Until two observations
exist there is nothing to reduce, so the yield is the zero value.
*/
type Governor struct {
	*core.PrimitiveError

	capacity  int
	reduction core.Primitive
	history   []float64
	out       float64
}

func NewGovernor(capacity int, reduction core.Primitive) *Governor {
	op := &Governor{PrimitiveError: core.NewPrimitiveError(), capacity: capacity, reduction: reduction}

	if capacity < 1 {
		op.Error(core.ErrShape)
	}

	return op
}

func (governor *Governor) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if governor.Error() != nil {
			return
		}

		for arriving := range in {
			val := *(*float64)(arriving)
			governor.history = append(governor.history, val)

			if len(governor.history) > governor.capacity {
				governor.history = append([]float64(nil), governor.history[len(governor.history)-governor.capacity:]...)
			}

			if len(governor.history) < 2 {
				governor.out = 0

				if !yield(unsafe.Pointer(&governor.out)) {
					return
				}

				continue
			}

			if governor.reduction != nil {
				redIn := func(yieldRed func(unsafe.Pointer) bool) {
					yieldRed(unsafe.Pointer(&governor.history))
				}

				for out := range governor.reduction.Next(redIn) {
					governor.out = *(*float64)(out)
				}
			}

			if !yield(unsafe.Pointer(&governor.out)) {
				return
			}
		}
	}
}
