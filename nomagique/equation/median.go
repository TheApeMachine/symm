package equation

import (
	"iter"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Median owns the central order statistic of one run. It averages the two central
members. Empty runs report a shape error.
*/
type Median[U core.Floating] struct {
	core.Base[U, U]
}

func NewMedian[U core.Floating]() *Median[U] {
	return &Median[U]{}
}

func (op *Median[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		var values []U

		for arriving := range in {
			values = append(values, arriving.Read())
		}

		if len(values) == 0 {
			op.Error(core.ErrShape)
			return
		}

		slices.Sort(values)
		count := len(values)

		if !yield(op.Carrier((values[(count-1)/2] + values[count/2]) / 2)) {
			return
		}
	}
}
