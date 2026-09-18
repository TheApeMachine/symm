package pumpdump

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Quote is one executable touch.
*/
type Quote struct {
	Symbol   string
	Bid, Ask float64
	At       int64
}

/*
Fill is one executed trade.
*/
type Fill struct {
	Symbol string
	Price  float64
	Qty    float64
	At     int64
}

/*
Assemble gathers keyed ticker or level-3 inputs into one touch quote.
*/
type Assemble struct {
	*core.PrimitiveError

	stream string
	out    Quote
}

func NewTickerAssemble() *Assemble {
	return &Assemble{PrimitiveError: core.NewPrimitiveError(), stream: "ticker"}
}

func NewTouchAssemble() *Assemble {
	return &Assemble{PrimitiveError: core.NewPrimitiveError(), stream: "level3"}
}

func (assemble *Assemble) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var quote Quote
		haveBid := false
		haveAsk := false

		for arriving := range in {
			input := (*core.Input[string, []string, any])(arriving)

			if input == nil {
				continue
			}

			if input.Origin != nil {
				quote.Symbol = input.Origin.Identity()
			}

			if input.Value == nil || len(input.Key) != 3 {
				continue
			}

			if input.Key[0] != assemble.stream || input.Key[1] != "data" {
				continue
			}

			switch input.Key[2] {
			case "bid":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				quote.Bid = value
				haveBid = true
			case "ask":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				quote.Ask = value
				haveAsk = true
			case "timestamp":
				value, isTime := (*input.Value).(int64)

				if !isTime {
					continue
				}

				quote.At = value
			}
		}

		if !haveBid || !haveAsk || quote.Bid <= 0 || quote.Ask <= 0 {
			return
		}

		assemble.out = quote

		if !yield(unsafe.Pointer(&assemble.out)) {
			return
		}
	}
}

/*
TradeAssemble gathers keyed trade inputs into one fill.
*/
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

			if input.Key[0] != "trade" || input.Key[1] != "data" {
				continue
			}

			switch input.Key[2] {
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
