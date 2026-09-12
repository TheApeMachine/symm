package logic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pick owns selection of one candidate. The predicate sees the held value and the
arrival as a pair and decides whether the arrival replaces what is held.
*/
type Pick struct {
	err       error
	predicate core.Primitive
	held      bool
	current   float64
	out       float64
	pair      [2]float64
}

func NewPick(predicate core.Primitive) core.Primitive {
	return &Pick{predicate: predicate}
}

func (op *Pick) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			val := *in

			if !op.held {
				op.held = true
				op.current = val
				op.out = val

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			take := false
			op.pair = [2]float64{val, op.current}

			for decision := range op.predicate.Next(func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&op.pair))
			}) {
				dec := (*bool)(decision)
				take = *dec
			}

			if take {
				op.current = val
			}

			op.out = op.current
			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Pick) Error(errs ...error) error {
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

	return op.err
}
