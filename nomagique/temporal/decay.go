package temporal

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Decay multiplies each arrival by a retention factor. The clock yields elapsed
time; the shape yields the factor for that elapsed time. A missing clock is
infinite elapsed time. A missing shape is linear retention, floored at zero.
*/
type Decay struct {
	*core.PrimitiveError
	clock      core.Primitive
	shape      core.Primitive
	linear     bool
	input      data.Map[string]
	clockInput data.Map[string]
	shapeInput data.Map[string]
	elapsedMap data.Map[float64]
	output     data.Map[float64]
}

func NewDecay(clock, shape core.Primitive) *Decay {
	elapsedMap := data.NewOutputMap()
	elapsedMap.Values["elapsed"] = 0

	output := data.NewOutputMap()
	output.Values["value"] = 0

	return &Decay{
		PrimitiveError: core.NewPrimitiveError(),
		clock:          clock,
		shape:          shape,
		linear:         shape == nil,
		input:          data.NewMap("value", "value"),
		clockInput:     data.NewMap("elapsed", "elapsed"),
		shapeInput:     data.NewMap("factor", "factor"),
		elapsedMap:     elapsedMap,
		output:         output,
	}
}

func (op *Decay) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			if op.clock == nil {
				op.output.Values["value"] = 0

				for range adapter.Next(data.NewValue(op.output)) {
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				if !yield(arriving) {
					return
				}

				continue
			}

			for range op.clock.Next(data.NewValue(adapter)) {
			}

			if err := op.clock.Error(); err != nil {
				op.Error(err)
				return
			}

			var clockValues data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.clockInput)) {
				clockValues = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			elapsed, elapsedOK := clockValues.Values["elapsed"]

			if !elapsedOK {
				op.Error(core.ErrNotHeld)
				return
			}

			factor := math.Max(0, 1-elapsed)

			if !op.linear && op.shape != nil {
				op.elapsedMap.Values["elapsed"] = elapsed

				for range adapter.Next(data.NewValue(op.elapsedMap)) {
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				for range op.shape.Next(data.NewValue(adapter)) {
				}

				if err := op.shape.Error(); err != nil {
					op.Error(err)
					return
				}

				var shapeValues data.Map[float64]

				for pointer := range adapter.Next(data.NewValue(op.shapeInput)) {
					shapeValues = *(*data.Map[float64])(pointer)
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				shapeFactor, factorOK := shapeValues.Values["factor"]

				if !factorOK {
					op.Error(core.ErrNotHeld)
					return
				}

				factor = shapeFactor
			}

			op.output.Values["value"] = value * factor

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
