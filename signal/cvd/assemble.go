package cvd

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Fill is one aggressive execution.
*/
type Fill struct {
	Symbol string
	Side   string
	Price  float64
	Qty    float64
	At     int64
}

/*
Assemble gathers keyed trade inputs into one fill.
*/
type Assemble struct {
	*core.PrimitiveError

	out Fill
}

func NewAssemble() *Assemble {
	return &Assemble{PrimitiveError: core.NewPrimitiveError()}
}

func (assemble *Assemble) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var fill Fill
		havePrice := false
		haveQty := false

		for arriving := range in {
			input := (*core.Input[string, []string, any])(arriving)

			if input == nil {
				continue
			}

			if input.Origin != nil {
				fill.Symbol = input.Origin.Identity()
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

				fill.Side = value
			case "price":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				fill.Price = value
				havePrice = true
			case "qty":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				fill.Qty = value
				haveQty = true
			case "timestamp":
				value, isTime := (*input.Value).(int64)

				if !isTime {
					continue
				}

				fill.At = value
			}
		}

		if !havePrice || !haveQty || fill.Price <= 0 || fill.Qty <= 0 {
			return
		}

		assemble.out = fill

		if !yield(unsafe.Pointer(&assemble.out)) {
			return
		}
	}
}
