package statistic

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
WeightedMoments owns weighted Welford statistics by merging summaries.

A summary is *[4]float64 {mass, squared mass, mean, m2}. Each arriving summary
is merged into the running summary, which is yielded after every merge. A
single observation x with weight w is the summary {w, w², x, 0}, so folding
observations and merging buckets are the same recurrence. Effective support
is mass² / squared mass.
*/
type WeightedMoments struct {
	*core.PrimitiveError
	out [4]float64
}

func NewWeightedMoments() *WeightedMoments {
	return &WeightedMoments{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *WeightedMoments) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			other := (*[4]float64)(arriving)

			if other[0] < 0 || other[1] < 0 {
				op.Error(fmt.Errorf("%w: weighted moments: mass must be non-negative", core.ErrDomain))
				return
			}

			if other[0] != 0 && op.out[0] == 0 {
				op.out = *other
			} else if other[0] != 0 {
				mass := op.out[0] + other[0]
				delta := other[2] - op.out[2]
				op.out[3] += other[3] + delta*delta*op.out[0]*other[0]/mass
				op.out[2] += delta * other[0] / mass
				op.out[0] = mass
				op.out[1] += other[1]
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
