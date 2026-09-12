package logic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Gate routes each arrival through one of two operations according to a
predicate. The predicate and the branches are themselves Primitives; Gate does
not snapshot a run in order to replay it.
*/
type Gate struct {
	err       error
	predicate core.Primitive
	pass      core.Primitive
	fail      core.Primitive
}

func NewGate(
	predicate core.Primitive,
	pass, fail core.Primitive,
) core.Primitive {
	return &Gate{
		predicate: predicate,
		pass:      pass,
		fail:      fail,
	}
}

func (op *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			once := func(yield func(unsafe.Pointer) bool) {
				yield(arriving)
			}

			selected := false

			for decision := range op.predicate.Next(once) {
				in := (*bool)(decision)
				selected = *in
			}

			branch := op.fail
			if selected {
				branch = op.pass
			}

			for out := range branch.Next(once) {
				if !yield(out) {
					return
				}
			}
		}
	}
}

func (op *Gate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}
	if op.predicate != nil {
		if err := op.predicate.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}
	if op.pass != nil {
		if err := op.pass.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}
	if op.fail != nil {
		if err := op.fail.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
