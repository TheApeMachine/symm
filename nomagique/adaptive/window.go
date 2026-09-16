package adaptive

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
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
	*core.PrimitiveError

	all, recent            statistic.Moments
	observations, capacity float64
	out                    WindowReading
}

func NewWindow() *Window {
	return &Window{PrimitiveError: core.NewPrimitiveError(), capacity: 0}
}

func (window *Window) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			window.observations++
			window.capacity++
			reading := WindowReading{
				All: window.all.Update(val), Recent: window.recent.Update(val),
				Value: val, Observations: window.observations, ShedRatio: 1,
			}

			if window.observations > 3 && window.recent.Count > window.capacity*0.5 {
				window.recent.Shed(0.5)
				reading.Recent.Summarize(window.recent)
			}

			reading.Variance = reading.All.Variance
			reading.RecentCount = window.recent.Count
			reading.PriorCount = window.capacity - reading.RecentCount

			if window.observations > 3 && reading.RecentCount > 1 && reading.PriorCount > 1 && reading.Variance > 0 {
				shift := MeanShift{
					Variance:     reading.Variance,
					Observations: window.observations,
					RecentCount:  reading.RecentCount,
					PriorCount:   reading.PriorCount,
				}
				bound := shift.Bound()

				if math.Abs(window.recent.Mean-window.all.Mean) > bound {
					capacity := math.Max(1, math.Floor(window.capacity*0.5))
					reading.ShedRatio = capacity / window.capacity
					window.capacity = capacity
					window.all.Shed(reading.ShedRatio)
					reading.All.Summarize(window.all)
					window.recent = statistic.Moments{}
					reading.Recent = statistic.MomentReading{}
				}
			}

			reading.Capacity = window.capacity
			window.out = reading

			if !yield(unsafe.Pointer(&window.out)) {
				return
			}
		}
	}
}
