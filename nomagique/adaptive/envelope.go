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
Envelope replaces a value with the inclusive moment interval when the
estimator has dispersion.
*/
type Envelope struct {
	err         error
	moments     core.Primitive
	coefficient core.Primitive
	out         float64
}

func NewEnvelope(
	moments core.Primitive,
	coefficient core.Primitive,
) core.Primitive {
	return &Envelope{
		moments:     moments,
		coefficient: coefficient,
	}
}

func (op *Envelope) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for readingPtr := range op.moments.Next(in) {
			current := *(*statistic.MomentReading)(readingPtr)
			value := current.Value

			if current.Count > 1 && current.Dispersion > 0 {
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

				margin := current.Dispersion * coeffVal
				lower := current.Mean - margin
				upper := current.Mean + margin

				if value < lower {
					value = lower
				} else if value > upper {
					value = upper
				}
			}

			op.out = value

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Envelope) Error(errs ...error) error {
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
