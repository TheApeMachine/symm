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
*/
type Project struct {
	*core.PrimitiveError
	store       core.Primitive
	epoch       string
	coordinates [][4]string
	batch       map[string][]float64
	pairs       []float64
}

/*
NewProject builds a projector writing into store under the given model epoch.
*/
func NewProject(store core.Primitive, epoch uint64, coordinates ...[4]string) *Project {
	op := &Project{
		PrimitiveError: core.NewPrimitiveError(),
		store:          store,
		epoch:          strconv.FormatUint(epoch, 10),
		coordinates:    coordinates,
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

			clear(op.batch)
			at := float64(measurement.At.UnixNano())
			peer := measurement.Meta("peer")

			for index, coordinate := range op.coordinates {
				label := coordinate[0]

				if coordinate[1] != "" {
					label += ":" + coordinate[1]
				}

				entry := data.Pull(measurement.Read(label))

				if entry == nil || entry.Metric == nil {
					op.Error(fmt.Errorf("%w: relation: metric %q not held", core.ErrNotHeld, label))
					return
				}

				raw := entry.Metric.Raw

				key := strings.Join([]string{
					measurement.Label, measurement.Source, coordinate[0], coordinate[1],
					peer, coordinate[2], coordinate[3], op.epoch,
				}, "|")

				op.pairs[2*index] = at
				op.pairs[2*index+1] = raw
				op.batch[key] = op.pairs[2*index : 2*index+2]
			}

			for range op.store.Next(data.NewValue(op.batch).Next(nil)) {
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
