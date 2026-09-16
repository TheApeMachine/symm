package temporal

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Elapsed subtracts int64 nanoseconds before conversion to seconds so epoch
magnitude cannot erase a small interval by cancellation.
*/
type Elapsed struct {
	*core.PrimitiveError

	out float64
}

func NewElapsed() *Elapsed {
	return &Elapsed{PrimitiveError: core.NewPrimitiveError()}
}

func (elapsed *Elapsed) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			interval := (*Interval)(arriving)
			elapsed.out = float64(interval.To-interval.From) / float64(time.Second)

			if !yield(unsafe.Pointer(&elapsed.out)) {
				return
			}
		}
	}
}
