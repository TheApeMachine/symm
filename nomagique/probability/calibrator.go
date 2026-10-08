package probability

import (
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Calibrator scores arriving samples against retained prior errors.
*/
type Calibrator struct {
	*core.PrimitiveError
	history   []float64
	retention core.Primitive
}

func NewCalibrator(retention core.Primitive) core.Primitive {
	return &Calibrator{
		PrimitiveError: core.NewPrimitiveError(),
		retention:      retention,
	}
}

func (op *Calibrator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)
			priorCount := float64(len(op.history))
			ready := 0.0
			score := 0.0

			if priorCount > 0 {
				ready = 1.0
				hits := 0.0

				for _, prior := range op.history {
					if prior > val {
						hits++
					}
				}

				score = hits / priorCount
			}

			if op.retention == nil {
				op.history = append(op.history, val)
			}

			if op.retention != nil {
				candidate := append(slices.Clone(op.history), val)
				var retained []float64

				for out := range op.retention.Next(data.NewValue(candidate).Next(nil)) {
					retained = *(*[]float64)(out)
				}

				if err := op.retention.Error(); err != nil {
					op.Error(err)
					return
				}

				op.history = retained
			}

			for value := range data.NewValue(score, ready, priorCount).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
