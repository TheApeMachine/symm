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
	input     data.Map[string]
	output    data.Map[float64]
}

func NewCalibrator(retention core.Primitive) *Calibrator {
	output := data.NewOutputMap()
	output.Values["value"] = 0
	output.Values["ready"] = 0
	output.Values["prior_count"] = 0

	return &Calibrator{
		PrimitiveError: core.NewPrimitiveError(),
		retention:      retention,
		input:          data.NewMap("value", "value"),
		output:         output,
	}
}

func (op *Calibrator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			val, ok := values.Values["value"]

			if !ok {
				op.Error(core.ErrNotHeld)
				return
			}

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

				for out := range op.retention.Next(data.NewValue(candidate)) {
					retained = *(*[]float64)(out)
				}

				if err := op.retention.Error(); err != nil {
					op.Error(err)
					return
				}

				op.history = retained
			}

			op.output.Values["value"] = score
			op.output.Values["ready"] = ready
			op.output.Values["prior_count"] = priorCount

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
