package probability

import (
	"errors"
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
CalibratorReading scores the arriving sample against retained prior errors
before appending it. Ready is false until a prior exists.
*/
type CalibratorReading struct {
	Value      float64
	Ready      bool
	PriorCount float64
}

/*
Calibrator owns that rank. Retention is a configured collection transform:
identity for all history, Tail for a bounded history.
*/
type Calibrator struct {
	err       error
	history   []float64
	retention core.Primitive
	out       CalibratorReading
}

func NewCalibrator(retention core.Primitive) core.Primitive {
	return &Calibrator{retention: retention}
}

func (op *Calibrator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)

			if math.IsNaN(val) || math.IsInf(val, 0) {
				op.err = errors.Join(op.err, core.ErrShape)
				continue
			}

			reading := CalibratorReading{
				PriorCount: float64(len(op.history)),
				Ready:      len(op.history) > 0,
			}

			if reading.Ready {
				hits := 0.0

				for _, prior := range op.history {
					if prior > val {
						hits++
					}
				}

				reading.Value = hits / reading.PriorCount
			}

			if op.retention != nil {
				candidate := append(slices.Clone(op.history), val)
				retainedEval := transport.NewEvaluate(op.retention)
				var retained []float64

				for out := range retainedEval.Next(transport.NewValues(candidate).Next(nil)) {
					retained = *(*[]float64)(out)
				}

				err := retainedEval.Error()

				if err != nil {
					op.err = errors.Join(op.err, err)
					return
				}

				op.history = retained
			} else {
				op.history = append(op.history, val)
			}

			op.out = reading

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Calibrator) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	if op.retention != nil {
		if err := op.retention.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
