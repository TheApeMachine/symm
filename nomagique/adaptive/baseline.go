package adaptive

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Baseline owns causal moments and observation-driven baseline tracking.
Arriving values are folded into running moments.
Yields center (baseline) and scale (dispersion).
*/
type Baseline struct {
	*core.PrimitiveError
	window  core.Primitive
	moments *statistic.Estimator
	shed    *statistic.Shed
}

func NewBaseline(window ...core.Primitive) core.Primitive {
	var w core.Primitive
	if len(window) > 0 && window[0] != nil {
		w = window[0]
	}

	moments := statistic.NewEstimator()

	return &Baseline{
		PrimitiveError: core.NewPrimitiveError(),
		window:         w,
		moments:        moments,
		shed:           statistic.NewShed(moments),
	}
}

func (op *Baseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			value := *(*float64)(arriving)

			var reading [10]float64
			for pointer := range op.moments.Next(data.NewValue(value).Next(nil)) {
				reading = *(*[10]float64)(pointer)
			}

			if err := op.moments.Error(); err != nil {
				op.Error(err)
				return
			}

			center := value

			if reading[3] > 0 {
				center = reading[4]
			}

			scale := reading[9]

			for val := range data.NewValue(center, scale).Next(nil) {
				if !yield(val) {
					return
				}
			}
		}
	}
}
