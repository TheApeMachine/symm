package adaptive

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Baseline owns causal moments and the configured observation-driven window.
*/
type Baseline struct {
	*core.PrimitiveError
	window      core.Primitive
	moments     statistic.Moments
	input       data.Map[string]
	windowInput data.Map[string]
	output      data.Map[float64]
}

func NewBaseline(window ...core.Primitive) core.Primitive {
	adaptiveWindow := NewWindow()

	if len(window) > 0 && window[0] != nil {
		adaptiveWindow = window[0]
	}

	output := data.NewOutputMap()
	output.Values["center"] = 0
	output.Values["scale"] = 0

	return &Baseline{
		PrimitiveError: core.NewPrimitiveError(),
		window:         adaptiveWindow,
		input:          data.NewMap("value", "value"),
		windowInput:    data.NewMap("shed_ratio", "shed_ratio"),
		output:         output,
	}
}

func (op *Baseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			value, ok := values.Values["value"]

			if !ok {
				if !yield(arriving) {
					return
				}
				continue
			}

			reading := op.moments.Update(value)

			for range op.window.Next(data.NewValue(adapter)) {
			}

			if err := op.window.Error(); err != nil {
				op.Error(err)
				return
			}

			var window data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.windowInput)) {
				window = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			shedRatio, held := window.Values["shed_ratio"]

			if !held {
				op.Error(core.ErrNotHeld)
				return
			}

			op.moments.Shed(shedRatio)

			if reading.Prior.Count == 0 {
				if !yield(arriving) {
					return
				}
				continue
			}

			scale := 0.0

			if reading.Prior.Count > 1 && reading.Prior.M2 > 0 {
				scale = math.Sqrt(reading.Prior.M2 / (reading.Prior.Count - 1))
			}

			op.output.Values["center"] = reading.Prior.Mean
			op.output.Values["scale"] = scale

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
