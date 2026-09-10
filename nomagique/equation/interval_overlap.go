package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Interval is a half-open (from, to] span in nanoseconds.
*/
type Interval struct {
	From int64
	To   int64
}

/*
IntervalPair is two intervals that may overlap.
*/
type IntervalPair struct {
	Left  Interval
	Right Interval
}

/*
IntervalOverlap is (left.from < right.to) AND (right.from < left.to).
Endpoints touching at a single instant do not overlap for (from,to] spans.
*/
type IntervalOverlap struct {
	core.Base[IntervalPair, bool]
}

func NewIntervalOverlap() *IntervalOverlap {
	return &IntervalOverlap{}
}

func (op *IntervalOverlap) Next(
	in iter.Seq[core.Primitive[IntervalPair, IntervalPair]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			pair := arriving.Read()
			overlap := pair.Left.From < pair.Right.To && pair.Right.From < pair.Left.To

			if !yield(op.Carrier(overlap)) {
				return
			}
		}
	}
}
