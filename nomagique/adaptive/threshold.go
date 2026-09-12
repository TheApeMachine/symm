package adaptive

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Threshold composes a moment estimator with a dispersion coefficient.
A source with no dispersion has threshold one.
*/
type Threshold struct {
	err         error
	moments     core.Primitive
	coefficient core.Primitive
	out         float64
}

func NewThreshold(
	moments core.Primitive,
	coefficient core.Primitive,
) core.Primitive {
	return &Threshold{
		moments:     moments,
		coefficient: coefficient,
	}
}

func (op *Threshold) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for readingPtr := range op.moments.Next(in) {
			current := *(*statistic.MomentReading)(readingPtr)
			coeffValEval := transport.NewEvaluate(op.coefficient)
			var coeffVal float64

			for out := range coeffValEval.Next(transport.NewValues(current.Count).Next(nil)) {
				coeffVal = *(*float64)(out)
			}

			err := coeffValEval.Error()

			if err != nil {
				op.err = errors.Join(op.err, err)
				return
			}

			if current.Dispersion > 0 {
				op.out = current.Dispersion * coeffVal
			} else {
				op.out = 1
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Threshold) Error(errs ...error) error {
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

	if op.coefficient != nil {
		if err := op.coefficient.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
