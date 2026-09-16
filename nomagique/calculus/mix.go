package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MixRecord is left, right, and the weight that interpolates them.
*/
type MixRecord struct {
	Left   float64
	Weight float64
	Right  float64
}

/*
Mix owns left + weight*(right-left). Zero preserves left; one selects right.
*/
type Mix struct {
	*core.PrimitiveError

	out float64
}

func NewMix() *Mix {
	return &Mix{PrimitiveError: core.NewPrimitiveError()}
}

func (mix *Mix) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			record := *(*MixRecord)(arriving)
			mix.out = record.Left + record.Weight*(record.Right-record.Left)

			if !yield(unsafe.Pointer(&mix.out)) {
				return
			}
		}
	}
}
