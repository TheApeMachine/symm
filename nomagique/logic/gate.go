package logic

import (
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
	*core.PrimitiveError
	predicate core.Primitive
	pass      core.Primitive
	fail      core.Primitive
}

func NewGate(
	predicate core.Primitive,
	pass, fail core.Primitive,
) *Gate {
	return &Gate{
		PrimitiveError: core.NewPrimitiveError(),
		predicate:      predicate,
		pass:           pass,
		fail:           fail,
	}
}

func (op *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.predicate == nil {
				op.Error(core.ErrShape)
				return
			}

			once := func(yieldOnce func(unsafe.Pointer) bool) {
				yieldOnce(arriving)
			}

			selected := false

			for decision := range op.predicate.Next(once) {
				if decision == nil {
					op.Error(core.ErrShape)
					return
				}

				selected = *(*bool)(decision)
			}

			if err := op.predicate.Error(); err != nil {
				op.Error(err)
				return
			}

			branch := op.fail

			if selected {
				branch = op.pass
			}

			if branch == nil {
				continue
			}

			for out := range branch.Next(once) {
				if !yield(out) {
					return
				}
			}

			if err := branch.Error(); err != nil {
				op.Error(err)
				return
			}
		}
	}
}
