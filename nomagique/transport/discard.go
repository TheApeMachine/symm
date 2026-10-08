package transport

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Discard consumes a run without handing anything over.
*/
type Discard struct {
	err error
}

func NewDiscard() core.Primitive {
	return &Discard{}
}

func (op *Discard) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(func(unsafe.Pointer) bool) {
		for range in {
		}
	}
}

func (op *Discard) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
