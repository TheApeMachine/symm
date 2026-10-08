package statistic

import (
	"errors"
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
	err error
	q   float64
	out float64
}

func NewQuantile(q float64) core.Primitive {
	return &Quantile{q: q}
}

func (op *Quantile) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values []float64

		for arriving := range in {
			values = append(values, *(*float64)(arriving))
		}

		if len(values) == 0 {
			op.err = errors.Join(op.err, core.ErrShape)
			return
		}

		if op.q < 0 || op.q > 1 {
			op.err = errors.Join(op.err, core.ErrShape)
			return
		}

		slices.Sort(values)

		position := op.q * float64(len(values)-1)
		lower := math.Floor(position)
		upper := math.Ceil(position)

		if lower == upper {
			op.out = values[int(lower)]
		} else {
			op.out = values[int(lower)]*(upper-position) + values[int(upper)]*(position-lower)
		}

		if !yield(unsafe.Pointer(&op.out)) {
			return
		}
	}
}

func (op *Quantile) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
