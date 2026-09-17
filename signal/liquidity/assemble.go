package liquidity

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Quote is one touch: bid/ask prices and displayed quantities at an event time.
*/
type Quote struct {
	Symbol         string
	Bid, Ask       float64
	BidQty, AskQty float64
	At             int64
}

/*
Assemble gathers a run of keyed ticker inputs into one touch quote.
*/
type Assemble struct {
	*core.PrimitiveError

	out Quote
}

func NewAssemble() *Assemble {
	return &Assemble{PrimitiveError: core.NewPrimitiveError()}
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

			if input.Key[0] != "ticker" || input.Key[1] != "data" {
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
			case "bid_qty":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				quote.BidQty = value
			case "ask_qty":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				quote.AskQty = value
			case "timestamp":
				value, isTime := (*input.Value).(int64)

				if !isTime {
					continue
				}

				quote.At = value
			}
		}

		if !haveBid || !haveAsk {
			return
		}

		if quote.Bid <= 0 || quote.Ask <= 0 {
			return
		}

		assemble.out = quote

		if !yield(unsafe.Pointer(&assemble.out)) {
			return
		}
	}
}
