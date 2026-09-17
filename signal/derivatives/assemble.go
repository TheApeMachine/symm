package derivatives

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Snapshot is one derivative/reference ticker observation.
*/
type Snapshot struct {
	Symbol string
	Last   float64
	Index  float64
	Mark   float64
	OI     float64
	At     int64
}

/*
Fill is one futures execution.
*/
type Fill struct {
	Symbol string
	Side   string
	Kind   string
	Price  float64
	Qty    float64
	At     int64
}

type Assemble struct {
	*core.PrimitiveError

	out Snapshot
}

func NewAssemble() *Assemble {
	return &Assemble{PrimitiveError: core.NewPrimitiveError()}
}

func (assemble *Assemble) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var snapshot Snapshot
		haveLast := false
		haveIndex := false

		for arriving := range in {
			input := (*core.Input[string, []string, any])(arriving)

			if input == nil {
				continue
			}

			if input.Origin != nil {
				snapshot.Symbol = input.Origin.Identity()
			}

			if input.Value == nil || len(input.Key) != 3 {
				continue
			}

			if input.Key[0] != "futures" || input.Key[1] != "data" {
				continue
			}

			switch input.Key[2] {
			case "last":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				snapshot.Last = value
				haveLast = true
			case "index_price":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				snapshot.Index = value
				haveIndex = true
			case "mark_price":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				snapshot.Mark = value
			case "open_interest":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				snapshot.OI = value
			case "timestamp":
				value, isTime := (*input.Value).(int64)

				if !isTime {
					continue
				}

				snapshot.At = value
			}
		}

		if !haveLast || !haveIndex || snapshot.Last < 0 || snapshot.Index <= 0 || snapshot.OI < 0 {
			return
		}

		assemble.out = snapshot

		if !yield(unsafe.Pointer(&assemble.out)) {
			return
		}
	}
}

type TradeAssemble struct {
	*core.PrimitiveError

	out Fill
}

func NewTradeAssemble() *TradeAssemble {
	return &TradeAssemble{PrimitiveError: core.NewPrimitiveError()}
}

func (assemble *TradeAssemble) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			if input.Key[0] != "futures" || input.Key[1] != "data" {
				continue
			}

			switch input.Key[2] {
			case "side":
				value, isString := (*input.Value).(string)

				if !isString {
					continue
				}

				fill.Side = value
			case "type":
				value, isString := (*input.Value).(string)

				if !isString {
					continue
				}

				fill.Kind = value
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
