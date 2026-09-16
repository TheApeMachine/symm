package statistic

import (
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Median owns the central order statistic of one run. It averages the two central
members. Empty runs report ErrShape.
*/
type Median struct {
	*core.PrimitiveError

	out float64
}

func NewMedian() *Median {
	return &Median{PrimitiveError: core.NewPrimitiveError()}
}

func (median *Median) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values []float64

		for arriving := range in {
			values = append(values, *(*float64)(arriving))
		}

		if len(values) == 0 {
			median.Error(core.ErrShape)
			return
		}

		slices.Sort(values)
		count := len(values)
		median.out = (values[(count-1)/2] + values[count/2]) / 2

		yield(unsafe.Pointer(&median.out))
	}
}
