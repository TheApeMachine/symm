package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Parallel distributes sequential arrivals from an inbound sequence across its
configured branch primitives. Arrival i maps to branch i. Each branch's yields
are streamed downstream.
*/
type Parallel struct {
	*core.PrimitiveError
	branches []core.Primitive
}

func NewParallel(branches ...core.Primitive) core.Primitive {
	return &Parallel{
		PrimitiveError: core.NewPrimitiveError(),
		branches:       branches,
	}
}

func (op *Parallel) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		numBranches := len(op.branches)

		if numBranches == 0 {
			return
		}

		index := 0

		for arriving := range in {
			branch := op.branches[index%numBranches]
			once := func(forward func(unsafe.Pointer) bool) {
				forward(arriving)
			}

			for out := range branch.Next(once) {
				if !yield(out) {
					return
				}
			}

			op.Error(branch.Error())
			index++
		}
	}
}
