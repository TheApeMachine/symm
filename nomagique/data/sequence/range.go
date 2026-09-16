package sequence

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Range enumerates [0, count) for each arriving count.
*/
type Range struct {
	*core.PrimitiveError

	out float64
}

func NewRange() *Range {
	return &Range{PrimitiveError: core.NewPrimitiveError()}
}

func (rangePrimitive *Range) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			count := int(*(*float64)(arriving))

			for index := 0; index < count; index++ {
				rangePrimitive.out = float64(index)

				if !yield(unsafe.Pointer(&rangePrimitive.out)) {
					return
				}
			}
		}
	}
}
