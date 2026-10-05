package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pick owns selection of one candidate. The predicate sees the held value and the
arrival as a pair and decides whether the arrival replaces what is held.
*/
type Pick struct {
	*core.PrimitiveError
	predicate core.Primitive
	held      bool
	current   float64
	out       float64
	pair      [2]float64
}

func NewPick(predicate core.Primitive) *Pick {
	return &Pick{
		PrimitiveError: core.NewPrimitiveError(),
		predicate:      predicate,
	}
}

func (op *Pick) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.predicate == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)

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

			for decision := range op.predicate.Next(func(yieldPair func(unsafe.Pointer) bool) {
				yieldPair(unsafe.Pointer(&op.pair))
			}) {
				if decision == nil {
					op.Error(core.ErrShape)
					return
				}

				take = *(*bool)(decision)
			}

			if err := op.predicate.Error(); err != nil {
				op.Error(err)
				return
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
