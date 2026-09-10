package pumpdump

import (
	"iter"
	"slices"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
QuantityTarget retains observed quantities under the adaptive window policy.
Its median is an explicit reduction of the retained tail.
*/
type QuantityTarget struct {
	core.Base[float64, float64]
	window  *adaptive.Window
	median  *equation.Median[float64]
	history []float64
}

func newQuantityTarget() *QuantityTarget {
	return &QuantityTarget{
		window: adaptive.NewWindow(),
		median: equation.NewMedian[float64](),
	}
}

func (op *QuantityTarget) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			value := arriving.Read()
			capacity := int(op.window.Observe(value).Capacity)
			op.history = append(op.history, value)

			if capacity >= 0 && len(op.history) > capacity {
				op.history = slices.Clone(op.history[len(op.history)-capacity:])
			}

			median, err := transport.Evaluate(op.median, transport.Values(op.history...))

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(median)) {
				return
			}
		}
	}
}
