package temporal

import (
	container "container/ring"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Velocity owns the previous observation in its native coordinates:
position and time -> velocity.

Time is expressed in seconds on the mathematical wire. The first observation
and non-advancing time have zero rate; the latest point is always retained.
*/
type Velocity struct {
	*core.PrimitiveError
	store  *container.Ring
	seen   bool
	input  data.Map[string]
	output data.Map[float64]
}

func NewVelocity() core.Primitive {
	output := data.NewOutputMap()
	output.Values["velocity"] = 0

	return &Velocity{
		PrimitiveError: core.NewPrimitiveError(),
		store:          container.New(2),
		input: data.NewMap(
			"position", "position",
			"time", "time",
		),
		output: output,
	}
}

func (op *Velocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			position, positionOK := values.Values["position"]
			at, timeOK := values.Values["time"]

			if !positionOK || !timeOK {
				if !yield(arriving) {
					return
				}

				continue
			}

			point := [2]float64{position, at}
			op.store.Value = point
			velocity := 0.0

			if op.seen {
				previous := op.store.Prev().Value.([2]float64)
				elapsed := point[1] - previous[1]

				if elapsed > 0 {
					velocity = (point[0] - previous[0]) / elapsed
				}
			}

			op.seen = true
			op.store = op.store.Next()
			op.output.Values["velocity"] = velocity

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
