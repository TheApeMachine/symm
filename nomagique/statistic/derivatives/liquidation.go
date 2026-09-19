package derivatives

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
LiquidationReading holds running liquidation and derivatives volume metrics.
*/
type LiquidationReading struct {
	Buy           float64
	Sell          float64
	Gross         float64
	Net           float64
	Signed        float64
	Share         float64
	TradeNotional float64
}

/*
Liquidation accumulates trade fills and liquidation volume.
No structs, pure Value closure.
*/
type Liquidation types.Value[*Fill, LiquidationReading]

func NewLiquidation() Liquidation {
	var state LiquidationReading

	return func(fill *Fill) LiquidationReading {
		if fill == nil {
			return state
		}

		notional := fill.Price * fill.Qty
		state.TradeNotional += notional

		if fill.Kind == "liquidation" {
			if fill.Side == "buy" {
				state.Buy += notional
			}
			if fill.Side == "sell" {
				state.Sell += notional
			}
		}

		state.Gross = state.Buy + state.Sell
		state.Net = state.Buy - state.Sell

		if state.Gross > 0 {
			state.Signed = state.Net / state.Gross
		}

		if state.TradeNotional > 0 {
			state.Share = state.Gross / state.TradeNotional
		}

		return state
	}
}

type LiquidationBuy types.Value[LiquidationReading, float64]

func NewLiquidationBuy() LiquidationBuy {
	return func(r LiquidationReading) float64 { return r.Buy }
}

type LiquidationSell types.Value[LiquidationReading, float64]

func NewLiquidationSell() LiquidationSell {
	return func(r LiquidationReading) float64 { return r.Sell }
}

type NetLiquidation types.Value[LiquidationReading, float64]

func NewNetLiquidation() NetLiquidation {
	return func(r LiquidationReading) float64 { return r.Net }
}

type GrossLiquidation types.Value[LiquidationReading, float64]

func NewGrossLiquidation() GrossLiquidation {
	return func(r LiquidationReading) float64 { return r.Gross }
}

type LiquidationSigned types.Value[LiquidationReading, float64]

func NewLiquidationSigned() LiquidationSigned {
	return func(r LiquidationReading) float64 { return r.Signed }
}

type LiquidationShare types.Value[LiquidationReading, float64]

func NewLiquidationShare() LiquidationShare {
	return func(r LiquidationReading) float64 { return r.Share }
}

type GrossTradeNotional types.Value[LiquidationReading, float64]

func NewGrossTradeNotional() GrossTradeNotional {
	return func(r LiquidationReading) float64 { return r.TradeNotional }
}
