package adaptive

import (
	"math"

	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
WindowReading is the immutable numeric result of one policy transition.
*/
type WindowReading struct {
	All, Recent                              statistic.MomentReading
	Value, Capacity, Observations, ShedRatio float64
	Variance, RecentCount, PriorCount        float64
}

/*
Window owns the all/recent moment approximation of the existing mean-shift policy.
*/
type Window struct {
	all, recent            statistic.Moments
	observations, capacity float64
}

func NewWindow() *Window {
	return &Window{capacity: 0}
}

func (op *Window) Step(val float64) WindowReading {
	op.observations++
	op.capacity++
	reading := WindowReading{
		All: op.all.Update(val), Recent: op.recent.Update(val),
		Value: val, Observations: op.observations, ShedRatio: 1,
	}

	if op.observations > 3 && op.recent.Count > op.capacity*0.5 {
		op.recent.Shed(0.5)
		reading.Recent.Summarize(op.recent)
	}

	reading.Variance = reading.All.Variance
	reading.RecentCount = op.recent.Count
	reading.PriorCount = op.capacity - reading.RecentCount

	if op.observations > 3 && reading.RecentCount > 1 && reading.PriorCount > 1 && reading.Variance > 0 {
		shift := MeanShift{
			Variance:     reading.Variance,
			Observations: op.observations,
			RecentCount:  reading.RecentCount,
			PriorCount:   reading.PriorCount,
		}
		bound := shift.Bound()

		if math.Abs(op.recent.Mean-op.all.Mean) > bound {
			capacity := math.Max(1, math.Floor(op.capacity*0.5))
			reading.ShedRatio = capacity / op.capacity
			op.capacity = capacity
			op.all.Shed(reading.ShedRatio)
			reading.All.Summarize(op.all)
			op.recent = statistic.Moments{}
			reading.Recent = statistic.MomentReading{}
		}
	}

	reading.Capacity = op.capacity
	return reading
}
