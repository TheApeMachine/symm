package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Member admits one symbol's arrivals into its own Path and publishes the
retained price path into a shared keyed path store, so pair stages can measure
the symbol against its peers without peers ever living on the Measurement.

The path store is any keyed-map Primitive over map[string][][2]float64
(store.NewKV[string, [][2]float64]), shared with the Pairs and Leads stages of
every symbol of one signal. Each arrival is the *data.Adapter; the stage reads
the native "at" (UnixNano) and "price", and publishes observation_count,
path_accepted, path_restated and path_from back into the adapter. Optional
retention is forwarded to the owned Path.
*/
type Member struct {
	*core.PrimitiveError
	label string
	paths core.Primitive
	path  core.Primitive
}

func NewMember(label string, paths core.Primitive, retention ...core.Primitive) core.Primitive {
	return &Member{
		PrimitiveError: core.NewPrimitiveError(),
		label:          label,
		paths:          paths,
		path:           NewPath(retention...),
	}
}

func (op *Member) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var at, price float64
		idx := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}
			if idx == 0 {
				at = *(*float64)(arriving)
			} else if idx == 1 {
				price = *(*float64)(arriving)
			}
			idx++
		}

		if price <= 0 {
			op.Error(core.ErrDomain)
			return
		}

		var reading [2][]float64
		sample := [2]float64{at, price}

		for pointer := range op.path.Next(data.NewValue(unsafe.Pointer(&sample)).Next(nil)) {
			reading = *(*[2][]float64)(pointer)
		}

		if err := op.path.Error(); err != nil {
			op.Error(err)
			return
		}

		if len(reading[0]) < 9 {
			op.Error(core.ErrShape)
			return
		}

		if reading[0][4] == 1 {
			observations := make([][2]float64, len(reading[1])/2)

			for index := range observations {
				observations[index] = [2]float64{reading[1][index*2], reading[1][index*2+1]}
			}
			
			payload := map[string][][2]float64{op.label: observations}
			for range op.paths.Next(data.NewValue(unsafe.Pointer(&payload)).Next(nil)) {
			}

			if err := op.paths.Error(); err != nil {
				op.Error(err)
				return
			}
		}

		observation_count := reading[0][3]
		path_accepted := reading[0][4]
		path_restated := reading[0][5]
		path_from := float64(0)
		if reading[0][6] == 1 {
			path_from = reading[0][7]
		}

		if !yield(unsafe.Pointer(&observation_count)) { return }
		if !yield(unsafe.Pointer(&path_accepted)) { return }
		if !yield(unsafe.Pointer(&path_restated)) { return }
		if !yield(unsafe.Pointer(&path_from)) { return }
	}
}
