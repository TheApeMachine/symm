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
	return &Gate{PrimitiveError: core.NewPrimitiveError(), predicate: predicate,
		pass: pass,
		fail: fail,
	}
}

func (gate *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			if gate.predicate != nil {
				if err := gate.predicate.Error(); err != nil {
					gate.Error(err)
				}
			}
			if gate.pass != nil {
				if err := gate.pass.Error(); err != nil {
					gate.Error(err)
				}
			}
			if gate.fail != nil {
				if err := gate.fail.Error(); err != nil {
					gate.Error(err)
				}
			}
		}()
		for arriving := range in {
			once := func(yield func(unsafe.Pointer) bool) {
				yield(arriving)
			}

			selected := false

			for decision := range gate.predicate.Next(once) {
				in := (*bool)(decision)
				selected = *in
			}

			branch := gate.fail
			if selected {
				branch = gate.pass
			}

			for out := range branch.Next(once) {
				if !yield(out) {
					return
				}
			}
		}
	}
}
