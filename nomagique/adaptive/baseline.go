package adaptive

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Baseline owns causal moments and the configured observation-driven window.
It composes an Estimator for the moments and a Shed over that Estimator for
the window's support policy.
*/
type Baseline struct {
	*core.PrimitiveError
	window      core.Primitive
	moments     core.Primitive
	shed        core.Primitive
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

	moments := statistic.NewEstimator()

	return &Baseline{
		PrimitiveError: core.NewPrimitiveError(),
		window:         adaptiveWindow,
		moments:        moments,
		shed:           statistic.NewShed(moments),
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
				op.Error(core.ErrNotHeld)
				return
			}

			var reading [10]float64

			for pointer := range op.moments.Next(data.NewValue(value)) {
				reading = *(*[10]float64)(pointer)
			}

			if err := op.moments.Error(); err != nil {
				op.Error(err)
				return
			}

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

			for range op.shed.Next(data.NewValue(shedRatio)) {
			}

			if err := op.shed.Error(); err != nil {
				op.Error(err)
				return
			}

			center := value

			if reading[3] > 0 {
				center = reading[4]
			}

			op.output.Values["center"] = center
			op.output.Values["scale"] = reading[9]

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
