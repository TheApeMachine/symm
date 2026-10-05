package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Bind replaces an Adapter's domain/native key binding while preserving its
Measurement and shared numeric outputs.
*/
type Bind struct {
	*core.PrimitiveError
	mapping Map[string]
	state   State
	bound   *Adapter
}

func NewBind(mapping Map[string]) core.Primitive {
	return &Bind{
		PrimitiveError: core.NewPrimitiveError(),
		mapping:        mapping,
		bound: &Adapter{
			PrimitiveError: core.NewPrimitiveError(),
			values:         NewOutputMap(),
		},
	}
}

func (op *Bind) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			parent := *(**Adapter)(arriving)

			if parent == nil || parent.state == nil {
				op.Error(core.ErrShape)
				return
			}

			op.state.input = op.mapping
			op.state.output = parent.state.output
			op.bound.measurement = parent.measurement
			op.bound.state = &op.state

			if !yield(unsafe.Pointer(&op.bound)) {
				return
			}
		}
	}
}
