package crosssection

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
ChangeBaseline evaluates an adaptive causal baseline over the signed fraction
published by ChangeCounts. Yields baseline (center), divergence (residual),
and z-score.
*/
type ChangeBaseline struct {
	*core.PrimitiveError
	baseline core.Primitive
}

func NewChangeBaseline() *ChangeBaseline {
	return &ChangeBaseline{
		PrimitiveError: core.NewPrimitiveError(),
		baseline:       adaptive.NewBaseline(adaptive.NewWindow()),
	}
}

func (op *ChangeBaseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			fraction := *(*float64)(arriving)
			var center, scale float64
			index := 0

			for pointer := range op.baseline.Next(data.NewValue(fraction).Next(nil)) {
				if index == 0 {
					center = *(*float64)(pointer)
				}

				if index == 1 {
					scale = *(*float64)(pointer)
				}

				index++
			}

			if err := op.baseline.Error(); err != nil {
				op.Error(err)
				return
			}

			residual := fraction - center
			zscore := 0.0

			if scale > 0 {
				zscore = residual / scale
			}

			for value := range data.NewValue(center, residual, zscore).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
