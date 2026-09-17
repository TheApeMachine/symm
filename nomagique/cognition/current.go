package cognition

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Current extracts the active evaluation context from an Association.
If a class is present, it yields the class (the most recent transition).
Otherwise, it yields the context (the initial sensory transition).
*/
type Current struct {
	*core.PrimitiveError
	out []byte
}

func NewCurrent() *Current {
	return &Current{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (current *Current) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if current.Error() != nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				continue
			}

			assoc := (*Association)(arriving)
			if len(assoc.Class) > 0 {
				current.out = assoc.Class
			}

			if len(assoc.Class) == 0 {
				current.out = assoc.Context
			}

			if len(current.out) > 0 {
				if !yield(unsafe.Pointer(&current.out)) {
					return
				}
			}
		}
	}
}
