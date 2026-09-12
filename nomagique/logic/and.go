package logic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
And owns one Boolean operation. Configuration supplies the value a run starts
from. What it hands over is the running conjunction after every arrival.
*/
type And struct {
	err error
	acc bool
	out bool
}

func NewAnd(current bool) core.Primitive {
	return &And{
		acc: current,
	}
}

func (op *And) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*bool)(arriving)
			op.acc = op.acc && *in
			op.out = op.acc

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *And) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
