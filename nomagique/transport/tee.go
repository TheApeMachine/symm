package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Tee taps an input stream to an off-ramp primitive while yielding each arriving
item downstream along the primary path unchanged.
*/
type Tee struct {
	*core.PrimitiveError
	offramp core.Primitive
}

func NewTee(offramp core.Primitive) *Tee {
	return &Tee{
		PrimitiveError: core.NewPrimitiveError(),
		offramp:        offramp,
	}
}

func (tee *Tee) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil || tee.Error() != nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				continue
			}

			if tee.offramp != nil {
				one := func(yieldBranch func(unsafe.Pointer) bool) {
					yieldBranch(arriving)
				}

				for range tee.offramp.Next(one) {
				}

				if err := tee.offramp.Error(); err != nil {
					tee.Error(err)
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
