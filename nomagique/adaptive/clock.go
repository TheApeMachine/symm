package adaptive

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Clock normalizes |value| by the estimator's inclusive mean and applies its
configured pace. A non-positive mean leaves the pace unscaled.
*/
type Clock struct {
	*core.PrimitiveError
	moments core.Primitive
	pace    core.Primitive
	input   data.Map[string]
	output  data.Map[float64]
}

func NewClock(moments core.Primitive, pace core.Primitive) core.Primitive {
	output := data.NewOutputMap()
	output.Values["clock"] = 0

	return &Clock{
		PrimitiveError: core.NewPrimitiveError(),
		moments:        moments,
		pace:           pace,
		input:          data.NewMap("value", "value"),
		output:         output,
	}
}

func (op *Clock) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.moments == nil || op.pace == nil {
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

			var current [10]float64

			for pointer := range op.moments.Next(data.NewValue(value)) {
				current = *(*[10]float64)(pointer)
			}

			if err := op.moments.Error(); err != nil {
				op.Error(err)
				return
			}

			var pace float64

			for pointer := range op.pace.Next(data.NewValue(current[6])) {
				pace = *(*float64)(pointer)
			}

			if err := op.pace.Error(); err != nil {
				op.Error(err)
				return
			}

			ratio := 1.0

			if current[1] > 0 {
				ratio = math.Abs(current[6]) / current[1]
			}

			op.output.Values["clock"] = ratio * pace

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
