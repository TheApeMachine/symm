package probability

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
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
	core.Base[float64, CalibratorReading]
	history   []float64
	retention core.Primitive[[]float64, []float64]
	finite    *logic.Finite[float64]
}

func NewCalibrator(retention core.Primitive[[]float64, []float64]) *Calibrator {
	return &Calibrator{retention: retention, finite: logic.NewFinite[float64]()}
}

func (op *Calibrator) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[CalibratorReading, CalibratorReading]] {
	return func(yield func(core.Primitive[CalibratorReading, CalibratorReading]) bool) {
		for arriving := range in {
			value := arriving.Read()
			defined, err := transport.Evaluate(op.finite, transport.Values(value))

			if err != nil {
				op.Error(err)
				return
			}

			if !defined {
				op.Error(core.ErrShape)
				continue
			}

			reading := CalibratorReading{
				PriorCount: float64(len(op.history)),
				Ready:      len(op.history) > 0,
			}

			if reading.Ready {
				hits := 0.0

				for _, prior := range op.history {
					if prior > value {
						hits++
					}
				}

				reading.Value = hits / float64(len(op.history))
			}

			history := append(append([]float64{}, op.history...), value)

			if op.retention != nil {
				history, err = transport.Evaluate(op.retention, transport.Values(history))

				if err != nil {
					op.Error(err)
					return
				}
			}

			op.history = history

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}
