package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MixRecord is left, right, and the weight that interpolates them.
*/
type MixRecord[U core.Floating] struct {
	Left   U
	Weight U
	Right  U
}

/*
Mix owns left + weight*(right-left). Zero preserves left; one selects right.
*/
type Mix[U core.Floating] struct {
	core.Base[MixRecord[U], U]
}

func NewMix[U core.Floating]() *Mix[U] {
	return &Mix[U]{}
}

func (op *Mix[U]) Next(
	in iter.Seq[core.Primitive[MixRecord[U], MixRecord[U]]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			record := arriving.Read()

			if !yield(op.Carrier(record.Left + record.Weight*(record.Right-record.Left))) {
				return
			}
		}
	}
}
