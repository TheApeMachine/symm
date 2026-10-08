package temporal

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Elapsed subtracts int64 nanoseconds before conversion to seconds so epoch
magnitude cannot erase a small interval by cancellation.
Operands arrive in order: [from, to]. Output is (to - from) in seconds.
*/
type Elapsed struct {
	*core.PrimitiveError
}

func NewElapsed() core.Primitive {
	return &Elapsed{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Elapsed) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [2]*float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if values[0] == nil {
				values[0] = (*float64)(arriving)
				continue
			}

			values[1] = (*float64)(arriving)
		}

		if values[0] == nil || values[1] == nil {
			return
		}

		out := (*values[1] - *values[0]) / float64(time.Second)

		for value := range data.NewValue(out).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
