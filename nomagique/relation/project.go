package relation

import (
	"fmt"
	"iter"
	"strconv"
	"strings"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Project splits one data.Measurement into the named per-coordinate
observations and writes them to an ObservationStore under one model epoch.

The caller names the coordinates it projects as [4]string{metric, side,
unit, timescale}; the metric is read from the Measurement through a
data.Adapter as "metric" or "metric:side". Symbol, source, and peer (metadata
"peer") are stamped from the Measurement. Every requested metric becomes an
independent observational fact; nothing is collapsed into a signal-level
scalar. A metric that cannot be read rejects the Measurement as a whole: the
error is recorded and nothing is written.

Each arrival is **data.Measurement; it is yielded back once written.
*/
type Project struct {
	*core.PrimitiveError
	store       core.Primitive
	epoch       string
	coordinates [][4]string
	request     data.Map[string]
	batch       map[string][]float64
	pairs       []float64
}

/*
NewProject builds a projector writing into store under the given model epoch.
*/
func NewProject(store core.Primitive, epoch uint64, coordinates ...[4]string) *Project {
	request := data.NewMap()

	for _, coordinate := range coordinates {
		label := coordinate[0]

		if coordinate[1] != "" {
			label += ":" + coordinate[1]
		}

		request.Values[label] = label
	}

	op := &Project{
		PrimitiveError: core.NewPrimitiveError(),
		store:          store,
		epoch:          strconv.FormatUint(epoch, 10),
		coordinates:    coordinates,
		request:        request,
		batch:          make(map[string][]float64, len(coordinates)),
		pairs:          make([]float64, 2*len(coordinates)),
	}

	if store == nil {
		op.Error(fmt.Errorf("%w: relation: project requires a store", core.ErrDomain))
	}

	return op
}

func (op *Project) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			measurement := *(**data.Measurement)(arriving)

			if measurement == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := data.NewAdapter(measurement, data.NewState(data.NewMap()))
			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.request)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(fmt.Errorf("%w: relation: measurement unreadable: %w", core.ErrDomain, err))
				return
			}

			clear(op.batch)
			at := float64(measurement.At.UnixNano())
			peer := measurement.Meta("peer")

			for index, coordinate := range op.coordinates {
				label := coordinate[0]

				if coordinate[1] != "" {
					label += ":" + coordinate[1]
				}

				raw, held := values.Values[label]

				if !held {
					op.Error(fmt.Errorf("%w: relation: metric %q not held", core.ErrNotHeld, label))
					return
				}

				key := strings.Join([]string{
					measurement.Label, measurement.Source, coordinate[0], coordinate[1],
					peer, coordinate[2], coordinate[3], op.epoch,
				}, "|")

				op.pairs[2*index] = at
				op.pairs[2*index+1] = raw
				op.batch[key] = op.pairs[2*index : 2*index+2]
			}

			for range op.store.Next(data.NewValue(op.batch)) {
			}

			if err := op.store.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
