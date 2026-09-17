package quote

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
FuturesTrade lifts one futures execution into keyed market inputs.
*/
type FuturesTrade struct {
	*core.PrimitiveError

	origin   *transport.Address[string]
	symbol   any
	side     any
	price    any
	qty      any
	kind     any
	at       any
	symbolIn core.Input[string, []string, any]
	sideIn   core.Input[string, []string, any]
	priceIn  core.Input[string, []string, any]
	qtyIn    core.Input[string, []string, any]
	kindIn   core.Input[string, []string, any]
	atIn     core.Input[string, []string, any]
}

func NewFuturesTrade() *FuturesTrade {
	return &FuturesTrade{
		PrimitiveError: core.NewPrimitiveError(),
		origin:         transport.NewAddress[string](),
	}
}

func (quote *FuturesTrade) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			data := *(*kraken.FuturesTradeData)(arriving)
			identity := data.Symbol

			if identity == "" {
				identity = data.ProductID
			}

			quote.origin.Identify(identity)
			quote.symbol = identity
			quote.symbolIn = *core.NewInput(
				quote.origin, core.Write, []string{"futures", "data", "symbol"}, &quote.symbol,
			)

			if !yield(unsafe.Pointer(&quote.symbolIn)) {
				return
			}

			quote.side = data.Side
			quote.sideIn = *core.NewInput(
				quote.origin, core.Write, []string{"futures", "data", "side"}, &quote.side,
			)

			if !yield(unsafe.Pointer(&quote.sideIn)) {
				return
			}

			quote.price = data.Price.Float64()
			quote.priceIn = *core.NewInput(
				quote.origin, core.Write, []string{"futures", "data", "price"}, &quote.price,
			)

			if !yield(unsafe.Pointer(&quote.priceIn)) {
				return
			}

			quote.qty = data.Qty
			quote.qtyIn = *core.NewInput(
				quote.origin, core.Write, []string{"futures", "data", "qty"}, &quote.qty,
			)

			if !yield(unsafe.Pointer(&quote.qtyIn)) {
				return
			}

			quote.kind = data.Type
			quote.kindIn = *core.NewInput(
				quote.origin, core.Write, []string{"futures", "data", "type"}, &quote.kind,
			)

			if !yield(unsafe.Pointer(&quote.kindIn)) {
				return
			}

			quote.at = data.Timestamp.UnixNano()
			quote.atIn = *core.NewInput(
				quote.origin, core.Write, []string{"futures", "data", "timestamp"}, &quote.at,
			)

			if !yield(unsafe.Pointer(&quote.atIn)) {
				return
			}
		}
	}
}
