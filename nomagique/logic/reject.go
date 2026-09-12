package logic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Reject owns explicit refusal. It records the configured reason, consumes the
run, and hands nothing over.
*/
type Reject struct {
	err    error
	reason error
}

func NewReject(reason error) core.Primitive {
	return &Reject{reason: reason}
}

func (op *Reject) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(func(unsafe.Pointer) bool) {
		op.Error(op.reason)

		for range in {
		}
	}
}

func (op *Reject) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
