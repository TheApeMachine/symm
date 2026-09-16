package probability

import (
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
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
	*core.PrimitiveError

	history   []float64
	retention core.Primitive
	out       CalibratorReading
}

func NewCalibrator(retention core.Primitive) *Calibrator {
	return &Calibrator{PrimitiveError: core.NewPrimitiveError(), retention: retention}
}

func (calibrator *Calibrator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			if calibrator.retention != nil {
				if err := calibrator.retention.Error(); err != nil {
					calibrator.Error(err)
				}
			}
		}()
		for arriving := range in {
			val := *(*float64)(arriving)

			if math.IsNaN(val) || math.IsInf(val, 0) {
				calibrator.Error(core.ErrShape)
				continue
			}

			reading := CalibratorReading{
				PriorCount: float64(len(calibrator.history)),
				Ready:      len(calibrator.history) > 0,
			}

			if reading.Ready {
				hits := 0.0

				for _, prior := range calibrator.history {
					if prior > val {
						hits++
					}
				}

				reading.Value = hits / reading.PriorCount
			}

			if calibrator.retention != nil {
				candidate := append(slices.Clone(calibrator.history), val)
				retainedEval := calibrator.retention
				var retained []float64

				for out := range retainedEval.Next(sequence.NewValues(candidate).Next(nil)) {
					retained = *(*[]float64)(out)
				}

				err := retainedEval.Error()

				if err != nil {
					calibrator.Error(err)
					return
				}

				calibrator.history = retained
			} else {
				calibrator.history = append(calibrator.history, val)
			}

			calibrator.out = reading

			if !yield(unsafe.Pointer(&calibrator.out)) {
				return
			}
		}
	}
}
