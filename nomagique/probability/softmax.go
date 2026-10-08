package probability

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Softmax evaluates shifted exponential normalization for one arrival.
*/
type Softmax struct {
	*core.PrimitiveError
}

func NewSoftmax() core.Primitive {
	return &Softmax{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Softmax) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [3]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 3 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 3 {
			op.Error(core.ErrShape)
			return
		}

		logit := values[0]
		shift := values[1]
		total := values[2]

		if total == 0 {
			op.Error(core.ErrDomain)
			return
		}

		probability := math.Exp(logit-shift) / total

		for value := range data.NewValue(probability).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
