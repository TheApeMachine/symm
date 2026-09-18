package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Tick assembles one run of keyed market inputs into a price observation.
*/
type Tick struct {
	*core.PrimitiveError

	out PriceObservation
}

func NewTick() *Tick {
	return &Tick{PrimitiveError: core.NewPrimitiveError()}
}

func (tick *Tick) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		var observation PriceObservation
		havePrice := false

		for arriving := range in {
			input := (*core.Input[string, []string, any])(arriving)

			if input == nil {
				continue
			}

			if input.Origin != nil {
				observation.Symbol = input.Origin.Identity()
			}

			if input.Value == nil {
				continue
			}

			if len(input.Key) == 3 && input.Key[0] == "ticker" && input.Key[1] == "data" && input.Key[2] == "last" {
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				observation.Value = value
				havePrice = true
				continue
			}

			if len(input.Key) == 3 && input.Key[0] == "ticker" && input.Key[1] == "data" && input.Key[2] == "timestamp" {
				value, isTime := (*input.Value).(int64)

				if !isTime {
					continue
				}

				observation.At = value
			}
		}

		if !havePrice {
			return
		}

		tick.out = observation

		if !yield(unsafe.Pointer(&tick.out)) {
			return
		}
	}
}
