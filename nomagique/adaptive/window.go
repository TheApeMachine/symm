package adaptive

import (
	"errors"
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
	err                    error
	all, recent            statistic.Moments
	observations, capacity float64
	out                    WindowReading
}

func NewWindow() core.Primitive {
	return &Window{capacity: 0}
}

func (op *Window) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
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
			op.out = reading

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Window) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
