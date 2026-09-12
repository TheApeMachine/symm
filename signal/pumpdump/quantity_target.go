package pumpdump

import (
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
QuantityTarget retains observed quantities under the adaptive window policy.
Its median is an explicit reduction of the retained tail.
*/
type QuantityTarget struct {
	err     error
	window  core.Primitive
	median  core.Primitive
	history []float64
}

func newQuantityTarget() *QuantityTarget {
	return &QuantityTarget{
		window: adaptive.NewWindow(),
		median: statistic.NewMedian(),
	}
}

func (op *QuantityTarget) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			value := *(*float64)(arriving)
			windowEval := transport.NewEvaluate(op.window)
			var window adaptive.WindowReading

			for out := range windowEval.Next(transport.NewValues(value).Next(nil)) {
				window = *(*adaptive.WindowReading)(out)
			}

			if err := windowEval.Error(); err != nil {
				op.err = err
				return
			}

			capacity := int(window.Capacity)
			op.history = append(op.history, value)

			if capacity >= 0 && len(op.history) > capacity {
				op.history = slices.Clone(op.history[len(op.history)-capacity:])
			}

			medianEval := transport.NewEvaluate(op.median)
			var median float64

			for out := range medianEval.Next(transport.NewValues(op.history...).Next(nil)) {
				median = *(*float64)(out)
			}

			if err := medianEval.Error(); err != nil {
				op.err = err
				return
			}

			if !yield(unsafe.Pointer(&median)) {
				return
			}
		}
	}
}

func (op *QuantityTarget) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
