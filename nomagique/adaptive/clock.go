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
Clock normalizes |value| by the estimator's inclusive mean and applies its
configured pace. A non-positive mean leaves the pace unscaled.
*/
type Clock struct {
	err     error
	moments core.Primitive
	pace    core.Primitive
	out     float64
}

func NewClock(
	moments core.Primitive,
	pace core.Primitive,
) core.Primitive {
	return &Clock{moments: moments, pace: pace}
}

func (op *Clock) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for readingPtr := range op.moments.Next(in) {
			current := *(*statistic.MomentReading)(readingPtr)
			paceValEval := transport.NewEvaluate(op.pace)
			var paceVal float64

			for out := range paceValEval.Next(transport.NewValues(current.Value).Next(nil)) {
				paceVal = *(*float64)(out)
			}

			err := paceValEval.Error()

			if err != nil {
				op.err = errors.Join(op.err, err)
				return
			}

			ratio := 1.0

			if current.Mean > 0 {
				ratio = math.Abs(current.Value) / current.Mean
			}

			op.out = ratio * paceVal

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Clock) Error(errs ...error) error {
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

	if op.pace != nil {
		if err := op.pace.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
