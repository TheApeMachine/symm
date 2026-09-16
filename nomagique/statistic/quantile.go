package statistic

import (
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Quantile reduces one arriving run of values to one linearly interpolated
sample quantile. Observations are local to a single Next delivery; the next
delivery begins an independent reduction.
*/
type Quantile struct {
	*core.PrimitiveError

	q   float64
	out float64
}

func NewQuantile(q float64) *Quantile {
	return &Quantile{PrimitiveError: core.NewPrimitiveError(), q: q}
}

func (quantile *Quantile) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values []float64

		for arriving := range in {
			values = append(values, *(*float64)(arriving))
		}

		if len(values) == 0 {
			quantile.Error(core.ErrShape)
			return
		}

		if quantile.q < 0 || quantile.q > 1 {
			quantile.Error(core.ErrShape)
			return
		}

		slices.Sort(values)

		position := quantile.q * float64(len(values)-1)
		lower := math.Floor(position)
		upper := math.Ceil(position)

		if lower == upper {
			quantile.out = values[int(lower)]
		} else {
			quantile.out = values[int(lower)]*(upper-position) + values[int(upper)]*(position-lower)
		}

		if !yield(unsafe.Pointer(&quantile.out)) {
			return
		}
	}
}
