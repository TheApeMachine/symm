package derivatives

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
LiquidationReading is cumulative liquidation accounting.
*/
type LiquidationReading struct {
	Buy, Sell, Gross, Net, TradeNotional float64
	Signed, Share                        float64
	HasSigned, HasShare                  bool
	Rate                                 float64
	HasRate                              bool
}

type liquidationPath struct {
	buy, sell, gross float64
	start            int64
	hasStart         bool
}

/*
Liquidation accounts futures executions and liquidation notional.
*/
type Liquidation struct {
	*core.PrimitiveError

	paths map[string]*liquidationPath
	out   LiquidationReading
}

func NewLiquidation() *Liquidation {
	return &Liquidation{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]*liquidationPath),
	}
}

func (liquidation *Liquidation) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			fill := *(*Fill)(arriving)
			notional := fill.Price * fill.Qty
			state := liquidation.paths[fill.Symbol]

			if state == nil {
				state = &liquidationPath{}
				liquidation.paths[fill.Symbol] = state
			}

			if !state.hasStart {
				state.start = fill.At
				state.hasStart = true
			}

			if fill.At < state.start {
				state.start = fill.At
			}

			state.gross += notional

			if fill.Kind == "liquidation" {
				if fill.Side == "buy" {
					state.buy += notional
				}

				if fill.Side == "sell" {
					state.sell += notional
				}
			}

			grossLiq := state.buy + state.sell
			reading := LiquidationReading{
				Buy:           state.buy,
				Sell:          state.sell,
				Gross:         grossLiq,
				Net:           state.buy - state.sell,
				TradeNotional: state.gross,
			}

			if grossLiq > 0 {
				reading.Signed = (state.buy - state.sell) / grossLiq
				reading.HasSigned = true
			}

			if state.gross > 0 {
				reading.Share = grossLiq / state.gross
				reading.HasShare = true
			}

			if state.hasStart && fill.At > state.start {
				duration := float64(fill.At-state.start) / 1e9

				if duration > 0 {
					reading.Rate = grossLiq / duration
					reading.HasRate = true
				}
			}

			liquidation.out = reading

			if !yield(unsafe.Pointer(&liquidation.out)) {
				return
			}
		}
	}
}
