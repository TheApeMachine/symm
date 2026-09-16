package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Convert owns one representation pass-through on the wire.
*/
type Convert struct {
	*core.PrimitiveError
}

func NewConvert() *Convert {
	return &Convert{PrimitiveError: core.NewPrimitiveError()}
}

func (convert *Convert) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}
