package probability

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Softmax owns the shifted exponential normalization of one run of logits.
*/
type Softmax struct {
	err error
	out float64
}

func NewSoftmax() core.Primitive {
	return &Softmax{}
}

func (op *Softmax) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var logits []float64

		for arriving := range in {
			val := *(*float64)(arriving)

			if math.IsNaN(val) || math.IsInf(val, 0) {
				op.err = errors.Join(op.err, core.ErrShape)
				return
			}

			logits = append(logits, val)
		}

		if len(logits) == 0 {
			op.err = errors.Join(op.err, core.ErrNotHeld)
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
			op.out = val / total

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Softmax) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
