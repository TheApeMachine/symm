package calculus

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Log owns one field operation. What it hands over is the natural logarithm of each
arrival, operating in-place on the wire pointer.
*/
type Log struct {
	err error
}

func NewLog() core.Primitive {
	return &Log{}
}

func (op *Log) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Log(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Log) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
