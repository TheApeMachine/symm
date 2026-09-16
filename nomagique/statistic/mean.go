package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Mean owns the running arithmetic mean.
*/
type Mean struct {
	*core.PrimitiveError

	count float64
	mean  float64
	out   float64
}

func NewMean() *Mean {
	return &Mean{PrimitiveError: core.NewPrimitiveError()}
}

func (mean *Mean) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		count, average := 0.0, 0.0

		for arriving := range in {
			val := *(*float64)(arriving)
			count++
			average += (val - average) / count
			mean.out = average

			if !yield(unsafe.Pointer(&mean.out)) {
				return
			}
		}
	}
}
