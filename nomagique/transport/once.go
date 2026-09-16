package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Once completes a configured prerequisite before the first arrival, then passes
input through unchanged. Construction and empty runs perform no work. A failed
prerequisite ends the stream and is never retried implicitly.
*/
type Once struct {
	*core.PrimitiveError
	prerequisite core.Primitive
	completed    bool
}

func NewOnce(prerequisite core.Primitive) *Once {
	return &Once{
		PrimitiveError: core.NewPrimitiveError(),
		prerequisite:   prerequisite,
	}
}

func (once *Once) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range input {
			if once.Error() != nil {
				return
			}

			if !once.completed {
				for range once.prerequisite.Next(nil) {
				}

				if err := once.prerequisite.Error(); err != nil {
					once.Error(err)
					return
				}

				once.completed = true
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
