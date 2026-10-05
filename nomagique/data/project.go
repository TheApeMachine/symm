package data

import (
	"iter"
	"sort"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Project publishes selected Adapter outputs as one WORM Measurement. Aliases map
output metric labels to numeric state keys.
*/
type Project struct {
	*core.PrimitiveError
	arena      *ArenaOwner
	source     string
	aliases    Map[string]
	units      map[string]Unit
	timescales map[string]Timescale
	labels     []string
	metrics    []Metric
	out        *Measurement
}

func NewProject(
	arena *ArenaOwner,
	source string,
	aliases Map[string],
	units map[string]Unit,
	timescales map[string]Timescale,
) core.Primitive {
	labels := make([]string, 0, len(aliases.Values))

	for label := range aliases.Values {
		labels = append(labels, label)
	}

	sort.Strings(labels)

	return &Project{
		PrimitiveError: core.NewPrimitiveError(),
		arena:          arena,
		source:         source,
		aliases:        aliases,
		units:          units,
		timescales:     timescales,
		labels:         labels,
	}
}

func (op *Project) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.arena == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**Adapter)(arriving)

			if adapter == nil || adapter.measurement == nil || adapter.state == nil {
				op.Error(core.ErrShape)
				return
			}

			prior := adapter.measurement
			out := op.arena.NewMeasurement()
			out.Epoch = prior.Epoch
			out.Label = prior.Label
			out.Source = op.source
			out.SeqIdx = prior.SeqIdx
			out.Tick = prior.Tick
			out.At = prior.At
			out.From = prior.From
			op.metrics = op.metrics[:0]

			if from, ok := adapter.state.output.Values["cvd_epoch_from"]; ok {
				seconds := int64(from)
				nanoseconds := int64((from - float64(seconds)) * float64(time.Second))
				out.From = time.Unix(seconds, nanoseconds)
			}

			for _, label := range op.labels {
				stateKey := op.aliases.Values[label]
				value, ok := adapter.state.output.Values[stateKey]

				if !ok {
					continue
				}

				op.metrics = append(op.metrics, NewMetric(
					label,
					value,
					op.units[label],
					op.timescales[label],
				))
			}

			if err := adapter.Error(); err != nil {
				out.err = err
			}

			out.Write(op.metrics...)
			op.out = out

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
