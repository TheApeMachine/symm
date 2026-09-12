package temporal

import (
	"errors"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Timestamp converts a time.Time arrival to signed Unix nanoseconds.
*/
type Timestamp struct {
	err error
	out int64
}

func NewTimestamp() core.Primitive {
	return &Timestamp{}
}

func (op *Timestamp) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			op.out = (*time.Time)(arriving).UnixNano()

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Timestamp) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
