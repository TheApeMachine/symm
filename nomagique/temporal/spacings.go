package temporal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Spacings owns consecutive timestamp differences within one delivery run.
*/
type Spacings struct {
	*core.PrimitiveError

	previous int64
	seen     bool
	out      float64
}

func NewSpacings() *Spacings {
	return &Spacings{PrimitiveError: core.NewPrimitiveError()}
}

func (spacings *Spacings) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			at := *(*int64)(arriving)

			if spacings.seen {
				spacings.out = float64(at - spacings.previous)

				if !yield(unsafe.Pointer(&spacings.out)) {
					return
				}
			}

			spacings.previous, spacings.seen = at, true
		}
	}
}
