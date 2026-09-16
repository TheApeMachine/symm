package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
LessEqual owns one ordering relation. Pairing is external: what arrives is already
two values.
*/
type LessEqual struct {
	*core.PrimitiveError

	out bool
}

func NewLessEqual() *LessEqual {
	return &LessEqual{PrimitiveError: core.NewPrimitiveError()}
}

func (lessEqual *LessEqual) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*[2]float64)(arriving)
			lessEqual.out = in[0] <= in[1]

			if !yield(unsafe.Pointer(&lessEqual.out)) {
				return
			}
		}
	}
}
