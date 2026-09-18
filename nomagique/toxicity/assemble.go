package toxicity

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Quote is one touch observation.
*/
type Quote struct {
	Symbol         string
	Bid, Ask       float64
	BidQty, AskQty float64
	At             int64
}

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
Assemble gathers keyed level-3 inputs into one touch quote.
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

			if input.Key[0] != "level3" || input.Key[1] != "data" {
				continue
			}

			switch input.Key[2] {
			case "symbol":
				if s, ok := (*input.Value).(string); ok {
					quote.Symbol = s
				}
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
TradeAssemble gathers trade and optional touch keys into one fill with the
last observed touch prices and quantities when present in the same run.
*/
type TradeAssemble struct {
	*core.PrimitiveError

	out mixed
}

type mixed struct {
	Fill     Fill
	Quote    Quote
	HasFill  bool
	HasQuote bool
}

func NewTradeAssemble() *TradeAssemble {
	return &TradeAssemble{PrimitiveError: core.NewPrimitiveError()}
}

func (assemble *TradeAssemble) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var arrival mixed
		havePrice := false
		haveQty := false
		haveBid := false
		haveAsk := false

		for arriving := range in {
			input := (*core.Input[string, []string, any])(arriving)

			if input == nil {
				continue
			}

			if input.Origin != nil {
				arrival.Fill.Symbol = input.Origin.Identity()
				arrival.Quote.Symbol = input.Origin.Identity()
			}

			if input.Value == nil || len(input.Key) != 3 {
				continue
			}

			if input.Key[1] != "data" {
				continue
			}

			switch {
			case input.Key[0] == "trade" && input.Key[2] == "side":
				value, isString := (*input.Value).(string)

				if !isString {
					continue
				}

				arrival.Fill.Side = value
			case input.Key[0] == "trade" && input.Key[2] == "price":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				arrival.Fill.Price = value
				havePrice = true
			case input.Key[0] == "trade" && input.Key[2] == "qty":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				arrival.Fill.Qty = value
				haveQty = true
			case input.Key[0] == "trade" && input.Key[2] == "timestamp":
				value, isTime := (*input.Value).(int64)

				if !isTime {
					continue
				}

				arrival.Fill.At = value
			case input.Key[0] == "level3" && input.Key[2] == "bid":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				arrival.Quote.Bid = value
				haveBid = true
			case input.Key[0] == "level3" && input.Key[2] == "ask":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				arrival.Quote.Ask = value
				haveAsk = true
			case input.Key[0] == "level3" && input.Key[2] == "bid_qty":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				arrival.Quote.BidQty = value
			case input.Key[0] == "level3" && input.Key[2] == "ask_qty":
				value, isFloat := (*input.Value).(float64)

				if !isFloat {
					continue
				}

				arrival.Quote.AskQty = value
			case input.Key[0] == "level3" && input.Key[2] == "timestamp":
				value, isTime := (*input.Value).(int64)

				if !isTime {
					continue
				}

				arrival.Quote.At = value
			}
		}

		arrival.HasFill = havePrice && haveQty && arrival.Fill.Price > 0 && arrival.Fill.Qty > 0
		arrival.HasQuote = haveBid && haveAsk && arrival.Quote.Bid > 0 && arrival.Quote.Ask > 0

		if !arrival.HasFill && !arrival.HasQuote {
			return
		}

		assemble.out = arrival

		if !yield(unsafe.Pointer(&assemble.out)) {
			return
		}
	}
}
