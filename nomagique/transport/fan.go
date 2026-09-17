package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Fan presents one input run to every configured branch and streams what each
branch yields.
*/
type Fan struct {
	*core.PrimitiveError
	branches []core.Primitive
}

func NewFan(branches ...core.Primitive) *Fan {
	return &Fan{PrimitiveError: core.NewPrimitiveError(), branches: branches}
}

func (fan *Fan) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {
			for _, branch := range fan.branches {
				if err := branch.Error(); err != nil {
					fan.Error(err)
				}
			}
		}()

		for arriving := range in {
			for _, branch := range fan.branches {
				one := func(yieldBranch func(unsafe.Pointer) bool) {
					yieldBranch(arriving)
				}

				for out := range branch.Next(one) {
					if !yield(out) {
						return
					}
				}
			}
		}
	}
}
