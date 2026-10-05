package temporal

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
EnergyRates owns r² / elapsed seconds over interval arrivals.
*/
type EnergyRates struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewEnergyRates() *EnergyRates {
	output := data.NewOutputMap()
	output.Values["energy_rate"] = 0

	return &EnergyRates{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"value", "value",
			"from", "from",
			"to", "to",
		),
		output: output,
	}
}

func (op *EnergyRates) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			value, valueOK := values.Values["value"]
			from, fromOK := values.Values["from"]
			to, toOK := values.Values["to"]

			if !valueOK || !fromOK || !toOK {
				op.Error(core.ErrNotHeld)
				return
			}

			elapsed := (to - from) / float64(time.Second)

			if elapsed <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			op.output.Values["energy_rate"] = (value * value) / elapsed

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
