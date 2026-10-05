package adaptive

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Baseline owns causal moments and the configured observation-driven window.
It yields *[2]float64{center, scale}.
*/
type Baseline struct {
	*core.PrimitiveError
	window  *Window
	moments statistic.Moments
}

func NewBaseline(window ...*Window) core.Primitive {
	w := NewWindow()
	if len(window) > 0 && window[0] != nil {
		w = window[0]
	}

	return &Baseline{
		PrimitiveError: core.NewPrimitiveError(),
		window:         w,
	}
}

func (op *Baseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			val := *(*float64)(arriving)
			reading := op.moments.Update(val)

			w := op.window.Step(val)
			op.moments.Shed(w.ShedRatio)

			center := val
			if reading.Prior.Count > 0 {
				center = reading.Prior.Mean
			}

			out := [2]float64{center, reading.Dispersion}

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
