package quote

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker lifts one venue ticker payload into keyed market inputs.
*/
type Ticker struct {
	*core.PrimitiveError

	origin      *transport.Address[string]
	symbol      any
	last        any
	at          any
	bid         any
	ask         any
	bidQty      any
	askQty      any
	volume      any
	vwap        any
	low         any
	high        any
	change      any
	pct         any
	trades      any
	symbolInput core.Input[string, []string, any]
	lastInput   core.Input[string, []string, any]
	atInput     core.Input[string, []string, any]
	bidInput    core.Input[string, []string, any]
	askInput    core.Input[string, []string, any]
	bidQtyInput core.Input[string, []string, any]
	askQtyInput core.Input[string, []string, any]
	volumeInput core.Input[string, []string, any]
	vwapInput   core.Input[string, []string, any]
	lowInput    core.Input[string, []string, any]
	highInput   core.Input[string, []string, any]
	changeInput core.Input[string, []string, any]
	pctInput    core.Input[string, []string, any]
	tradesInput core.Input[string, []string, any]
}

func NewTicker() *Ticker {
	return &Ticker{
		PrimitiveError: core.NewPrimitiveError(),
		origin:         transport.NewAddress[string](),
	}
}

func (quote *Ticker) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			data := *(*kraken.TickerData)(arriving)
			quote.origin.Identify(data.Symbol)
			quote.symbol = data.Symbol
			quote.symbolInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"ticker", "data", "symbol"}, &quote.symbol,
			)

			if !yield(unsafe.Pointer(&quote.symbolInput)) {
				return
			}

			if data.Last != nil {
				quote.last = data.Last.Float64()
				quote.lastInput = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"ticker", "data", "last"}, &quote.last,
				)

				if !yield(unsafe.Pointer(&quote.lastInput)) {
					return
				}
			}

			quote.at = data.Timestamp.UnixNano()
			quote.atInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"ticker", "data", "timestamp"}, &quote.at,
			)

			if !yield(unsafe.Pointer(&quote.atInput)) {
				return
			}

			if data.Bid != nil {
				quote.bid = data.Bid.Float64()
				quote.bidInput = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"ticker", "data", "bid"}, &quote.bid,
				)

				if !yield(unsafe.Pointer(&quote.bidInput)) {
					return
				}
			}

			if data.Ask != nil {
				quote.ask = data.Ask.Float64()
				quote.askInput = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"ticker", "data", "ask"}, &quote.ask,
				)

				if !yield(unsafe.Pointer(&quote.askInput)) {
					return
				}
			}

			quote.bidQty = data.BidQty
			quote.bidQtyInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"ticker", "data", "bid_qty"}, &quote.bidQty,
			)

			if !yield(unsafe.Pointer(&quote.bidQtyInput)) {
				return
			}

			quote.askQty = data.AskQty
			quote.askQtyInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"ticker", "data", "ask_qty"}, &quote.askQty,
			)

			if !yield(unsafe.Pointer(&quote.askQtyInput)) {
				return
			}

			quote.volume = data.Volume
			quote.volumeInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"ticker", "data", "volume"}, &quote.volume,
			)

			if !yield(unsafe.Pointer(&quote.volumeInput)) {
				return
			}

			quote.vwap = data.Vwap
			quote.vwapInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"ticker", "data", "vwap"}, &quote.vwap,
			)

			if !yield(unsafe.Pointer(&quote.vwapInput)) {
				return
			}

			if data.Low != nil {
				quote.low = data.Low.Float64()
				quote.lowInput = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"ticker", "data", "low"}, &quote.low,
				)

				if !yield(unsafe.Pointer(&quote.lowInput)) {
					return
				}
			}

			if data.High != nil {
				quote.high = data.High.Float64()
				quote.highInput = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"ticker", "data", "high"}, &quote.high,
				)

				if !yield(unsafe.Pointer(&quote.highInput)) {
					return
				}
			}

			if data.Change != nil {
				quote.change = data.Change.Float64()
				quote.changeInput = *core.NewInput[string, []string, any](
					quote.origin, core.Write, []string{"ticker", "data", "change"}, &quote.change,
				)

				if !yield(unsafe.Pointer(&quote.changeInput)) {
					return
				}
			}

			quote.pct = data.ChangePct
			quote.pctInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"ticker", "data", "change_pct"}, &quote.pct,
			)

			if !yield(unsafe.Pointer(&quote.pctInput)) {
				return
			}

			if data.Trades == nil {
				continue
			}

			quote.trades = *data.Trades
			quote.tradesInput = *core.NewInput[string, []string, any](
				quote.origin, core.Write, []string{"ticker", "data", "trades"}, &quote.trades,
			)

			if !yield(unsafe.Pointer(&quote.tradesInput)) {
				return
			}
		}
	}
}
