package quote

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Futures lifts one futures ticker snapshot into keyed market inputs.
*/
type Futures struct {
	*core.PrimitiveError

	origin   *transport.Address[string]
	symbol   any
	last     any
	index    any
	mark     any
	oi       any
	at       any
	symbolIn core.Input[string, []string, any]
	lastIn   core.Input[string, []string, any]
	indexIn  core.Input[string, []string, any]
	markIn   core.Input[string, []string, any]
	oiIn     core.Input[string, []string, any]
	atIn     core.Input[string, []string, any]
}

func NewFutures() *Futures {
	return &Futures{
		PrimitiveError: core.NewPrimitiveError(),
		origin:         transport.NewAddress[string](),
	}
}

func (quote *Futures) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			data := *(*kraken.FuturesTickerData)(arriving)
			identity := data.Symbol

			if identity == "" {
				identity = data.ProductID
			}

			quote.origin.Identify(identity)
			quote.symbol = identity
			quote.symbolIn = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"futures", "data", "symbol"}, &quote.symbol,
			)

			if !yield(unsafe.Pointer(&quote.symbolIn)) {
				return
			}

			quote.at = data.Timestamp.UnixNano()
			quote.atIn = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"futures", "data", "timestamp"}, &quote.at,
			)

			if !yield(unsafe.Pointer(&quote.atIn)) {
				return
			}

			if data.Last != nil {
				quote.last = data.Last.Float64()
				quote.lastIn = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"futures", "data", "last"}, &quote.last,
				)

				if !yield(unsafe.Pointer(&quote.lastIn)) {
					return
				}
			}

			if data.IndexPrice != nil {
				quote.index = data.IndexPrice.Float64()
				quote.indexIn = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"futures", "data", "index_price"}, &quote.index,
				)

				if !yield(unsafe.Pointer(&quote.indexIn)) {
					return
				}
			}

			if data.MarkPrice != nil {
				quote.mark = data.MarkPrice.Float64()
				quote.markIn = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"futures", "data", "mark_price"}, &quote.mark,
				)

				if !yield(unsafe.Pointer(&quote.markIn)) {
					return
				}
			}

			quote.oi = data.OpenInterest
			quote.oiIn = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"futures", "data", "open_interest"}, &quote.oi,
			)

			if !yield(unsafe.Pointer(&quote.oiIn)) {
				return
			}
		}
	}
}
