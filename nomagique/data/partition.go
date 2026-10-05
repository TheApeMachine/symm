package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Partition owns independent Primitive graphs per Measurement label.

A Step run may contain several Adapter bindings for one measurement. Partition
selects the label from the first Adapter, presents that entire run to the
label's graph without buffering it, and keeps every estimator state isolated
from other labels.
*/
type Partition struct {
	*core.PrimitiveError
	factory   func() core.Primitive
	pipelines map[string]core.Primitive
}

func NewPartition(factory func() core.Primitive) core.Primitive {
	return &Partition{
		PrimitiveError: core.NewPrimitiveError(),
		factory:        factory,
		pipelines:      make(map[string]core.Primitive),
	}
}

func (op *Partition) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.factory == nil {
			op.Error(core.ErrNotHeld)
			return
		}

		next, stop := iter.Pull(in)
		defer stop()

		first, ok := next()

		if !ok {
			return
		}

		adapter := *(**Adapter)(first)

		if adapter == nil || adapter.measurement == nil {
			op.Error(core.ErrShape)
			return
		}

		label := adapter.measurement.Label
		pipeline := op.pipelines[label]

		if pipeline == nil {
			pipeline = op.factory()
			op.pipelines[label] = pipeline
		}

		run := func(forward func(unsafe.Pointer) bool) {
			if !forward(first) {
				return
			}

			for {
				arriving, available := next()

				if !available {
					return
				}

				current := *(**Adapter)(arriving)

				if current == nil || current.measurement == nil || current.measurement.Label != label {
					op.Error(core.ErrShape)
					return
				}

				if !forward(arriving) {
					return
				}
			}
		}

		for out := range pipeline.Next(run) {
			if !yield(out) {
				return
			}
		}

		if err := pipeline.Error(); err != nil {
			op.Error(err)
		}
	}
}
