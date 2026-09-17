package quote

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Touch lifts one accepted Level 3 touch into keyed market inputs.
*/
type Touch struct {
	*core.PrimitiveError

	origin      *transport.Address[string]
	symbol      any
	bid         any
	ask         any
	bidQty      any
	askQty      any
	at          any
	symbolInput core.Input[string, []string, any]
	bidInput    core.Input[string, []string, any]
	askInput    core.Input[string, []string, any]
	bidQtyInput core.Input[string, []string, any]
	askQtyInput core.Input[string, []string, any]
	atInput     core.Input[string, []string, any]
}

func NewTouch() *Touch {
	return &Touch{
		PrimitiveError: core.NewPrimitiveError(),
		origin:         transport.NewAddress[string](),
	}
}

func (quote *Touch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			data := *(*kraken.Level3Touch)(arriving)
			quote.origin.Identify(data.Symbol)
			quote.symbol = data.Symbol
			quote.symbolInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"level3", "data", "symbol"}, &quote.symbol,
			)

			if !yield(unsafe.Pointer(&quote.symbolInput)) {
				return
			}

			quote.at = data.Timestamp.UnixNano()
			quote.atInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"level3", "data", "timestamp"}, &quote.at,
			)

			if !yield(unsafe.Pointer(&quote.atInput)) {
				return
			}

			if data.Bid != nil {
				quote.bid = data.Bid.Float64()
				quote.bidInput = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"level3", "data", "bid"}, &quote.bid,
				)

				if !yield(unsafe.Pointer(&quote.bidInput)) {
					return
				}
			}

			if data.Ask != nil {
				quote.ask = data.Ask.Float64()
				quote.askInput = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"level3", "data", "ask"}, &quote.ask,
				)

				if !yield(unsafe.Pointer(&quote.askInput)) {
					return
				}
			}

			if data.BidQty != nil {
				quote.bidQty = data.BidQty.Float64()
				quote.bidQtyInput = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"level3", "data", "bid_qty"}, &quote.bidQty,
				)

				if !yield(unsafe.Pointer(&quote.bidQtyInput)) {
					return
				}
			}

			if data.AskQty != nil {
				quote.askQty = data.AskQty.Float64()
				quote.askQtyInput = *core.NewInput(
					quote.origin, core.Write, []string{"level3", "data", "ask_qty"}, &quote.askQty,
				)

				if !yield(unsafe.Pointer(&quote.askQtyInput)) {
					return
				}
			}
		}
	}
}
