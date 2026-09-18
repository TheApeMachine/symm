package hawkes

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Assemble gathers keyed trade inputs into one Hawkes event.
*/
type Assemble struct {
	*core.PrimitiveError

	out Event
}

func NewAssemble() *Assemble {
	return &Assemble{PrimitiveError: core.NewPrimitiveError()}
}

func (assemble *Assemble) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var event Event
		haveSide := false

		for arriving := range in {
			input := (*core.Input[string, []string, any])(arriving)

			if input == nil {
				continue
			}

			if input.Origin != nil {
				event.Symbol = input.Origin.Identity()
			}

			if input.Value == nil || len(input.Key) != 3 {
				continue
			}

			if input.Key[0] != "trade" || input.Key[1] != "data" {
				continue
			}

			switch input.Key[2] {
			case "side":
				value, isString := (*input.Value).(string)

				if !isString {
					continue
				}

				event.Side = value
				haveSide = true
			case "timestamp":
				value, isTime := (*input.Value).(int64)

				if !isTime {
					continue
				}

				event.At = value
			}
		}

		if !haveSide || (event.Side != "buy" && event.Side != "sell") {
			return
		}

		assemble.out = event

		if !yield(unsafe.Pointer(&assemble.out)) {
			return
		}
	}
}
