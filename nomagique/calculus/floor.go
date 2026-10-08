package calculus

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Floor owns one field operation. What it hands over is the floor of each
arrival, operating in-place on the wire pointer.
*/
type Floor struct {
	err error
}

func NewFloor() core.Primitive {
	return &Floor{}
}

func (op *Floor) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Floor(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Floor) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
