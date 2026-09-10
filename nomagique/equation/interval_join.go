package equation

import (
	"fmt"
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

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
	core.Base[IntervalJoinInput, IntervalPair]
}

func NewIntervalJoin() *IntervalJoin {
	return &IntervalJoin{}
}

func (op *IntervalJoin) Next(
	in iter.Seq[core.Primitive[IntervalJoinInput, IntervalJoinInput]],
) iter.Seq[core.Primitive[IntervalPair, IntervalPair]] {
	return func(yield func(core.Primitive[IntervalPair, IntervalPair]) bool) {
		for arriving := range in {
			input := arriving.Read()

			if err := ordered(input.Left); err != nil {
				op.Error(err)
				return
			}

			if err := ordered(input.Right); err != nil {
				op.Error(err)
				return
			}

			left, right := 0, 0

			for left < len(input.Left) && right < len(input.Right) {
				a, b := input.Left[left], input.Right[right]

				if a.From < b.To && b.From < a.To {
					if !yield(op.Carrier(IntervalPair{Left: a, Right: b})) {
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
