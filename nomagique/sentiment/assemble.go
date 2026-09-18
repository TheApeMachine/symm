package sentiment

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Tick is one last-price observation.
*/
type Tick struct {
	Symbol string
	Last   float64
	At     int64
}

type Assemble struct {
	*core.PrimitiveError

	out Tick
}

func NewAssemble() *Assemble {
	return &Assemble{PrimitiveError: core.NewPrimitiveError()}
}

func (assemble *Assemble) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var tick Tick
		haveLast := false

		for arriving := range in {
			input := (*core.Input[string, []string, any])(arriving)

			if input == nil {
				continue
			}

			if input.Origin != nil {
				tick.Symbol = input.Origin.Identity()
			}

			if input.Value == nil || len(input.Key) != 3 {
				continue
			}

			if input.Key[0] != "ticker" || input.Key[1] != "data" {
				continue
			}

			switch input.Key[2] {
			case "last":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				tick.Last = value
				haveLast = true
			case "timestamp":
				value, isTime := (*input.Value).(int64)

				if !isTime {
					continue
				}

				tick.At = value
			}
		}

		if !haveLast || tick.Last <= 0 {
			return
		}

		assemble.out = tick

		if !yield(unsafe.Pointer(&assemble.out)) {
			return
		}
	}
}
