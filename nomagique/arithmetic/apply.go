package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Apply aligns one arrival with each configured operation and forwards that
operation's complete output. Operations retain their identity between runs, so
each coordinate may own an independent recurrence.
*/
type Apply struct {
	*core.PrimitiveError

	operations []core.Primitive
}

func NewApply(operations ...core.Primitive) *Apply {
	return &Apply{PrimitiveError: core.NewPrimitiveError(), operations: operations}
}

func (apply *Apply) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			for _, operation := range apply.operations {
				if err := operation.Error(); err != nil {
					apply.Error(err)
				}
			}
		}()
		index := 0

		for arriving := range in {
			if index >= len(apply.operations) {
				apply.Error(core.ErrShape)
				return
			}

			once := func(y func(unsafe.Pointer) bool) {
				y(arriving)
			}

			for out := range apply.operations[index].Next(once) {
				if !yield(out) {
					return
				}
			}

			apply.Error(apply.operations[index].Error())
			index++
		}

		if index != len(apply.operations) {
			apply.Error(core.ErrShape)
		}
	}
}
