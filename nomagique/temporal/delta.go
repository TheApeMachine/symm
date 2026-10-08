package temporal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Delta owns the previous observation and yields the difference (current - previous).
The first observation yields 0.0.
Each arrival is *float64; it yields *float64.
*/
type Delta struct {
	*core.PrimitiveError
	seen      bool
	prevValue float64
	out       float64
}

func NewDelta() core.Primitive {
	return &Delta{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Delta) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			value := *(*float64)(arriving)

			if !op.seen {
				op.seen = true
				op.prevValue = value
				op.out = 0.0

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			op.out = value - op.prevValue
			op.prevValue = value

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
