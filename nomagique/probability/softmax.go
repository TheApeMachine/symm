package probability

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Softmax owns the shifted exponential normalization of one run of logits.
*/
type Softmax struct {
	*core.PrimitiveError

	out float64
}

func NewSoftmax() *Softmax {
	return &Softmax{PrimitiveError: core.NewPrimitiveError()}
}

func (softmax *Softmax) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var logits []float64

		for arriving := range in {
			val := *(*float64)(arriving)

			if math.IsNaN(val) || math.IsInf(val, 0) {
				softmax.Error(core.ErrShape)
				return
			}

			logits = append(logits, val)
		}

		if len(logits) == 0 {
			softmax.Error(core.ErrNotHeld)
			return
		}

		shift := logits[0]

		for _, logit := range logits[1:] {
			if logit > shift {
				shift = logit
			}
		}

		var total float64
		shifted := make([]float64, len(logits))

		for index, logit := range logits {
			shifted[index] = math.Exp(logit - shift)
			total += shifted[index]
		}

		for _, val := range shifted {
			softmax.out = val / total

			if !yield(unsafe.Pointer(&softmax.out)) {
				return
			}
		}
	}
}
