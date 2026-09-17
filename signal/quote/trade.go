package quote

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Trade lifts one venue trade payload into keyed market inputs.
*/
type Trade struct {
	*core.PrimitiveError

	origin      *transport.Address[string]
	symbol      any
	side        any
	price       any
	qty         any
	at          any
	symbolInput core.Input[string, []string, any]
	sideInput   core.Input[string, []string, any]
	priceInput  core.Input[string, []string, any]
	qtyInput    core.Input[string, []string, any]
	atInput     core.Input[string, []string, any]
}

func NewTrade() *Trade {
	return &Trade{
		PrimitiveError: core.NewPrimitiveError(),
		origin:         transport.NewAddress[string](),
	}
}

func (quote *Trade) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			data := *(*kraken.TradeData)(arriving)
			quote.origin.Identify(data.Symbol)
			quote.symbol = data.Symbol
			quote.symbolInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"trade", "data", "symbol"}, &quote.symbol,
			)

			if !yield(unsafe.Pointer(&quote.symbolInput)) {
				return
			}

			quote.side = data.Side
			quote.sideInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"trade", "data", "side"}, &quote.side,
			)

			if !yield(unsafe.Pointer(&quote.sideInput)) {
				return
			}

			quote.price = data.Price.Float64()
			quote.priceInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"trade", "data", "price"}, &quote.price,
			)

			if !yield(unsafe.Pointer(&quote.priceInput)) {
				return
			}

			quote.qty = data.Qty
			quote.qtyInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"trade", "data", "qty"}, &quote.qty,
			)

			if !yield(unsafe.Pointer(&quote.qtyInput)) {
				return
			}

			quote.at = data.Timestamp.UnixNano()
			quote.atInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"trade", "data", "timestamp"}, &quote.at,
			)

			if !yield(unsafe.Pointer(&quote.atInput)) {
				return
			}
		}
	}
}
