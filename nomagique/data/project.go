package data

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Project is the numeric egress boundary. It selects named domain values from an
Adapter's shared output state, writes only the values that are actually
defined, and finalizes one Measurement.

Project owns no signal mathematics. Metric declarations supplied at
construction provide only names, units, and timescales.
*/
type Project struct {
	*core.PrimitiveError
	arena   *ArenaOwner
	metrics []Metric
	buffer  []Metric
	from    time.Time
}

func NewProject(arena *ArenaOwner, metrics ...Metric) core.Primitive {
	return &Project{
		PrimitiveError: core.NewPrimitiveError(),
		arena:          arena,
		metrics:        metrics,
		buffer:         make([]Metric, 0, len(metrics)),
	}
}

func (op *Project) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**Adapter)(arriving)

			if adapter == nil || adapter.measurement == nil || adapter.state == nil || op.arena == nil {
				op.Error(core.ErrShape)
				return
			}

			source := adapter.measurement

			if op.from.IsZero() {
				op.from = source.At
			}

			op.buffer = op.buffer[:0]

			for _, declared := range op.metrics {
				value, ok := adapter.state.output.Values[declared.Label]

				if !ok {
					continue
				}

				metric := declared
				metric.Raw = value
				op.buffer = append(op.buffer, metric)
			}

			out := op.arena.NewMeasurement()
			out.Epoch = source.Epoch
			out.Label = source.Label
			out.Source = op.arena.source
			out.SeqIdx = source.SeqIdx
			out.Tick = source.Tick
			out.At = source.At
			out.From = op.from
			out.Write(op.buffer...)

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
