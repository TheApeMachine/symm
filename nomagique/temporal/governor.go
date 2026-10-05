package temporal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Governor retains a tail of arrivals whose length is the configured capacity
and hands that tail, as one collection, to a reduction. Until two observations
exist there is nothing to reduce, so the yield is the zero value.
*/
type Governor struct {
	*core.PrimitiveError
	capacity  int
	reduction core.Primitive
	history   []float64
	input     data.Map[string]
	output    data.Map[float64]
}

func NewGovernor(capacity int, reduction core.Primitive) *Governor {
	output := data.NewOutputMap()
	output.Values["value"] = 0

	op := &Governor{
		PrimitiveError: core.NewPrimitiveError(),
		capacity:       capacity,
		reduction:      reduction,
		input:          data.NewMap("value", "value"),
		output:         output,
	}

	if capacity < 1 {
		op.Error(core.ErrShape)
	}

	return op
}

func (op *Governor) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			op.history = append(op.history, value)

			if len(op.history) > op.capacity {
				op.history = append([]float64(nil), op.history[len(op.history)-op.capacity:]...)
			}

			if len(op.history) < 2 {
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

			reduced := 0.0

			if op.reduction != nil {
				for out := range op.reduction.Next(data.NewValue(op.history)) {
					reduced = *(*float64)(out)
				}

				if err := op.reduction.Error(); err != nil {
					op.Error(err)
					return
				}
			}

			op.output.Values["value"] = reduced

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
