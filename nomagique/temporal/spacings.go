package temporal

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Spacings owns consecutive timestamp differences within one delivery run.
*/
type Spacings struct {
	err      error
	previous int64
	seen     bool
	out      float64
}

func NewSpacings() core.Primitive {
	return &Spacings{}
}

func (op *Spacings) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			at := *(*int64)(arriving)

			if op.seen {
				op.out = float64(at - op.previous)

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}
			}

			op.previous, op.seen = at, true
		}
	}
}

func (op *Spacings) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
