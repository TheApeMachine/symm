package cognition

import (
	"bytes"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Associate transforms sequential transitions into empirical associations.
When the first transition arrives, it yields an association with no class to observe sensory context.
When subsequent transitions arrive, it yields Association{Context: precursor, Class: current}.
*/
type Associate struct {
	*core.PrimitiveError
	precursor []byte
	out       Association
}

func NewAssociate() *Associate {
	return &Associate{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (associate *Associate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if associate.Error() != nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				continue
			}

			current := *(*[]byte)(arriving)
			if len(current) == 0 {
				continue
			}

			if len(associate.precursor) == 0 {
				associate.precursor = bytes.Clone(current)
				associate.out = Association{
					Context: bytes.Clone(current),
					Class:   nil,
				}

				if !yield(unsafe.Pointer(&associate.out)) {
					return
				}

				continue
			}

			associate.out = Association{
				Context: bytes.Clone(associate.precursor),
				Class:   bytes.Clone(current),
			}
			associate.precursor = bytes.Clone(current)

			if !yield(unsafe.Pointer(&associate.out)) {
				return
			}
		}
	}
}
