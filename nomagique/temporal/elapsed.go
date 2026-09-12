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
	err error
	out float64
}

func NewElapsed() core.Primitive {
	return &Elapsed{}
}

func (op *Elapsed) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			interval := (*Interval)(arriving)
			op.out = float64(interval.To-interval.From) / float64(time.Second)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Elapsed) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
