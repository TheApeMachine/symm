package temporal

import (
	"fmt"
	"iter"
	"unsafe"

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
	err error
	out bool
}

func NewIntervalOverlap() core.Primitive {
	return &IntervalOverlap{}
}

func (op *IntervalOverlap) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*IntervalPair)(arriving)
			op.out = pair.Left.From < pair.Right.To && pair.Right.From < pair.Left.To

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *IntervalOverlap) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}

/*
IntervalJoinInput is two ordered, internally disjoint interval paths.
*/
type IntervalJoinInput struct {
	Left  []Interval
	Right []Interval
}

/*
IntervalJoin emits overlapping pairs from two ordered paths. Touching endpoints
do not overlap. It advances the interval ending first, visiting O(left+right)
intervals instead of constructing a Cartesian product.
*/
type IntervalJoin struct {
	err error
	out IntervalPair
}

func NewIntervalJoin() core.Primitive {
	return &IntervalJoin{}
}

func (op *IntervalJoin) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*IntervalJoinInput)(arriving)

			if err := ordered(input.Left); err != nil {
				op.err = err
				return
			}

			if err := ordered(input.Right); err != nil {
				op.err = err
				return
			}

			left, right := 0, 0

			for left < len(input.Left) && right < len(input.Right) {
				a, b := input.Left[left], input.Right[right]

				if a.From < b.To && b.From < a.To {
					op.out = IntervalPair{Left: a, Right: b}

					if !yield(unsafe.Pointer(&op.out)) {
						return
					}
				}

				if a.To <= b.To {
					left++
				}

				if b.To <= a.To {
					right++
				}
			}
		}
	}
}

func (op *IntervalJoin) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}

func ordered(path []Interval) error {
	for index, interval := range path {
		if interval.From >= interval.To {
			return fmt.Errorf("%w: interval join interval %d must be positive", core.ErrShape, index)
		}

		if index > 0 && interval.From < path[index-1].To {
			return fmt.Errorf("%w: interval join interval %d overlaps its predecessor", core.ErrShape, index)
		}
	}

	return nil
}
