package adaptive

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Gating suppresses values inside a configured threshold of inclusive moments.
*/
type Gating struct {
	err       error
	moments   core.Primitive
	threshold core.Primitive
	out       float64
}

func NewGating(
	moments core.Primitive,
	threshold core.Primitive,
) core.Primitive {
	return &Gating{moments: moments, threshold: threshold}
}

func (op *Gating) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for readingPtr := range op.moments.Next(in) {
			current := *(*statistic.MomentReading)(readingPtr)
			limitEval := transport.NewEvaluate(op.threshold)
			var limit float64

			for out := range limitEval.Next(transport.NewValues(current.Count).Next(nil)) {
				limit = *(*float64)(out)
			}

			err := limitEval.Error()

			if err != nil {
				op.err = errors.Join(op.err, err)
				return
			}

			op.out = current.Value

			if current.Dispersion > 0 && math.Abs(current.Value-current.Mean) < limit {
				op.out = 0
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Gating) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	if op.moments != nil {
		if err := op.moments.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	if op.threshold != nil {
		if err := op.threshold.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
