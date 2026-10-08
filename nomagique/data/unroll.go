package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type Unroll struct {
	*core.PrimitiveError
}

func NewUnroll() core.Primitive {
	return &Unroll{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Unroll) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := (*Value[unsafe.Pointer])(arriving)
			for ptr := range val.Next(nil) {
				if !yield(ptr) {
					return
				}
			}
		}
	}
}
