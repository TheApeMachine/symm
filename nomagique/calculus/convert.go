package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Convert owns representation pass-through on the wire.
*/
type Convert struct {
	*core.PrimitiveError
}

func NewConvert() *Convert {
	return &Convert{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Convert) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
