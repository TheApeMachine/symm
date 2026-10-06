package adaptive

import (
	"errors"
	"iter"
	"math"
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
	shift                  core.Primitive
	input                  data.Map[string]
	shiftInput             data.Map[string]
	output                 data.Map[float64]
}

func NewWindow() core.Primitive {
	output := data.NewOutputMap()
	output.Values["capacity"] = 0
	output.Values["observations"] = 0
	output.Values["shed_ratio"] = 1
	output.Values["variance"] = 0
	output.Values["recent_count"] = 0
	output.Values["prior_count"] = 0

	all := statistic.NewEstimator()
	recent := statistic.NewEstimator()

	return &Window{
		PrimitiveError: core.NewPrimitiveError(),
		all:            all,
		recent:         recent,
		allShed:        statistic.NewShed(all),
		recentShed:     statistic.NewShed(recent),
		shift:          NewMeanShift(),
		input:          data.NewMap("value", "value"),
		shiftInput:     data.NewMap("bound", "bound"),
		output:         output,
	}
}

func (op *Window) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			value, ok := values.Values["value"]

			if !ok {
				op.Error(core.ErrNotHeld)
				return
			}

			op.observations++
			op.capacity++
			var all, recent [10]float64

			for pointer := range op.all.Next(data.NewValue(value)) {
				all = *(*[10]float64)(pointer)
			}

			for pointer := range op.recent.Next(data.NewValue(value)) {
				recent = *(*[10]float64)(pointer)
			}

			if err := errors.Join(op.all.Error(), op.recent.Error()); err != nil {
				op.Error(err)
				return
			}

			shedRatio := 1.0
			recentCount := recent[0]

			if op.observations > 3 && recentCount > op.capacity*0.5 {
				for pointer := range op.recentShed.Next(data.NewValue(0.5)) {
					recentCount = (*(*[3]float64)(pointer))[0]
				}

				if err := op.recentShed.Error(); err != nil {
					op.Error(err)
					return
				}
			}

			variance := all[8]
			priorCount := op.capacity - recentCount

			op.output.Values["capacity"] = op.capacity
			op.output.Values["observations"] = op.observations
			op.output.Values["shed_ratio"] = shedRatio
			op.output.Values["variance"] = variance
			op.output.Values["recent_count"] = recentCount
			op.output.Values["prior_count"] = priorCount

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if op.observations > 3 && recentCount > 1 && priorCount > 1 && variance > 0 {
				for range op.shift.Next(data.NewValue(adapter)) {
				}

				if err := op.shift.Error(); err != nil {
					op.Error(err)
					return
				}

				var shift data.Map[float64]

				for pointer := range adapter.Next(data.NewValue(op.shiftInput)) {
					shift = *(*data.Map[float64])(pointer)
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				bound, held := shift.Values["bound"]

				if !held {
					op.Error(core.ErrNotHeld)
					return
				}

				if math.Abs(recent[1]-all[1]) > bound {
					capacity := math.Max(1, math.Floor(op.capacity*0.5))
					shedRatio = capacity / op.capacity
					op.capacity = capacity

					for range op.allShed.Next(data.NewValue(shedRatio)) {
					}

					if err := op.allShed.Error(); err != nil {
						op.Error(err)
						return
					}

					op.recent = statistic.NewEstimator()
					op.recentShed = statistic.NewShed(op.recent)
				}
			}

			op.output.Values["capacity"] = op.capacity
			op.output.Values["shed_ratio"] = shedRatio

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
