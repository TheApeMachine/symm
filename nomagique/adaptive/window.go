package adaptive

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
)

/*
Window owns the all/recent moment approximation of the existing mean-shift
policy. It does not claim to implement the full ADWIN algorithm. Its state is
fixed numeric storage; no record maps or computation graphs grow per observation.
*/
type Window struct {
	core.Base[float64, WindowReading]
	all, recent            equation.Moments
	observations, capacity float64
	Reading                WindowReading
}

/*
WindowReading is the immutable numeric result of one policy transition.
*/
type WindowReading struct {
	All, Recent                              equation.MomentReading
	Value, Capacity, Observations, ShedRatio float64
	Variance, RecentCount, PriorCount        float64
}

func NewWindow() *Window {
	return &Window{capacity: 2}
}

func (op *Window) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[WindowReading, WindowReading]] {
	return func(yield func(core.Primitive[WindowReading, WindowReading]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(op.Observe(arriving.Read()))) {
				return
			}
		}
	}
}

/*
Observe advances the policy directly over unboxed numeric state.
*/
func (op *Window) Observe(value float64) WindowReading {
	op.observations++
	op.capacity++
	reading := WindowReading{
		All: op.all.Update(value), Recent: op.recent.Update(value),
		Value: value, Observations: op.observations, ShedRatio: 1,
	}

	if op.observations > 3 && op.recent.Count > op.capacity*0.5 {
		op.recent.Shed(0.5)
		reading.Recent.Summarize(op.recent)
	}

	reading.Variance = reading.All.Variance
	reading.RecentCount = op.recent.Count
	reading.PriorCount = op.capacity - reading.RecentCount

	if op.observations > 3 && reading.RecentCount > 1 && reading.PriorCount > 1 && reading.Variance > 0 {
		shift := equation.MeanShift{
			Variance: reading.Variance, Observations: op.observations,
			RecentCount: reading.RecentCount, PriorCount: reading.PriorCount,
		}
		bound := shift.Bound()

		if math.Abs(op.recent.Mean-op.all.Mean) > bound {
			capacity := math.Max(2, math.Floor(op.capacity*0.5))
			reading.ShedRatio = capacity / op.capacity
			op.capacity = capacity
			op.all.Shed(reading.ShedRatio)
			reading.All.Summarize(op.all)
			op.recent = equation.Moments{}
			reading.Recent = equation.MomentReading{}
		}
	}

	reading.Capacity = op.capacity
	op.Reading = reading
	return reading
}
