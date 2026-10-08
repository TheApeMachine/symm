package adaptive

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Window owns the all/recent moment approximation of the mean-shift policy. It
composes one Estimator for all observations and one for the recent run, each
with a Shed that reduces its support.
*/
type Window struct {
	*core.PrimitiveError
	all, recent            *statistic.Estimator
	allShed, recentShed    core.Primitive
	observations, capacity float64
}

func NewWindow() core.Primitive {
	all := statistic.NewEstimator()
	recent := statistic.NewEstimator()

	return &Window{
		PrimitiveError: core.NewPrimitiveError(),
		all:            all,
		recent:         recent,
		allShed:        statistic.NewShed(all),
		recentShed:     statistic.NewShed(recent),
	}
}

func (op *Window) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			value := *(*float64)(arriving)
			op.observations++
			op.capacity++
			var all, recent [10]float64

			for pointer := range op.all.Next(data.NewValue(value).Next(nil)) {
				all = *(*[10]float64)(pointer)
			}

			for pointer := range op.recent.Next(data.NewValue(value).Next(nil)) {
				recent = *(*[10]float64)(pointer)
			}

			if err := errors.Join(op.all.Error(), op.recent.Error()); err != nil {
				op.Error(err)
				return
			}

			shedRatio := 1.0
			recentCount := recent[0]

			if op.observations > 3 && recentCount > op.capacity*0.5 {
				for pointer := range op.recentShed.Next(data.NewValue(0.5).Next(nil)) {
					recentCount = (*(*[3]float64)(pointer))[0]
				}

				if err := op.recentShed.Error(); err != nil {
					op.Error(err)
					return
				}
			}

			_ = all

			for p := range data.NewValue(shedRatio).Next(nil) {
				if !yield(p) {
					return
				}
			}
		}
	}
}
