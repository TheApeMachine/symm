package adaptive

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Window owns the all/recent moment approximation of the mean-shift policy.
*/
type Window struct {
	*core.PrimitiveError
	all, recent            statistic.Moments
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

	return &Window{
		PrimitiveError: core.NewPrimitiveError(),
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
			all := op.all.Update(value)
			op.recent.Update(value)
			shedRatio := 1.0

			if op.observations > 3 && op.recent.Count > op.capacity*0.5 {
				op.recent.Shed(0.5)
			}

			variance := all.Variance
			recentCount := op.recent.Count
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

				if math.Abs(op.recent.Mean-op.all.Mean) > bound {
					capacity := math.Max(1, math.Floor(op.capacity*0.5))
					shedRatio = capacity / op.capacity
					op.capacity = capacity
					op.all.Shed(shedRatio)
					op.recent = statistic.Moments{}
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
