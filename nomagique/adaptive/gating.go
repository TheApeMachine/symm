package adaptive

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Gating suppresses values inside a configured support-dependent limit.
*/
type Gating struct {
	*core.PrimitiveError
	moments        core.Primitive
	threshold      core.Primitive
	input          data.Map[string]
	thresholdInput data.Map[string]
	count          data.Map[float64]
	output         data.Map[float64]
}

func NewGating(moments core.Primitive, threshold core.Primitive) core.Primitive {
	count := data.NewOutputMap()
	count.Values["count"] = 0
	output := data.NewOutputMap()
	output.Values["value"] = 0

	return &Gating{
		PrimitiveError: core.NewPrimitiveError(),
		moments:        moments,
		threshold:      threshold,
		input:          data.NewMap("value", "value"),
		thresholdInput: data.NewMap("scale", "scale"),
		count:          count,
		output:         output,
	}
}

func (op *Gating) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.moments == nil || op.threshold == nil {
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

			op.count.Values["count"] = current[0]

			for range adapter.Next(data.NewValue(op.count)) {
			}

			for range op.threshold.Next(data.NewValue(adapter)) {
			}

			if err := op.threshold.Error(); err != nil {
				op.Error(err)
				return
			}

			var threshold data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.thresholdInput)) {
				threshold = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			limit, held := threshold.Values["scale"]

			if !held {
				op.Error(core.ErrNotHeld)
				return
			}

			gated := current[6]

			if current[9] > 0 && math.Abs(current[6]-current[1]) < limit {
				gated = 0
			}

			op.output.Values["value"] = gated

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
