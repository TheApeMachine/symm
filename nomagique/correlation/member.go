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
	label  string
	paths  core.Primitive
	path   core.Primitive
	input  data.Map[string]
	output data.Map[float64]
}

func NewMember(label string, paths core.Primitive, retention ...core.Primitive) core.Primitive {
	return &Member{
		PrimitiveError: core.NewPrimitiveError(),
		label:          label,
		paths:          paths,
		path:           NewPath(retention...),
		input:          data.NewMap("at", "at", "price", "price"),
		output:         data.NewOutputMap(),
	}
}

func (op *Member) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
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

			at, atOK := values.Values["at"]
			price, priceOK := values.Values["price"]

			if !atOK || !priceOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if price <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			var reading [2][]float64

			for pointer := range op.path.Next(data.NewValue([2]float64{at, price})) {
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

				for range op.paths.Next(data.NewValue(map[string][][2]float64{
					op.label: observations,
				})) {
				}

				if err := op.paths.Error(); err != nil {
					op.Error(err)
					return
				}
			}

			clear(op.output.Values)
			op.output.Values["observation_count"] = reading[0][3]
			op.output.Values["path_accepted"] = reading[0][4]
			op.output.Values["path_restated"] = reading[0][5]

			if reading[0][6] == 1 {
				op.output.Values["path_from"] = reading[0][7]
			}

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
