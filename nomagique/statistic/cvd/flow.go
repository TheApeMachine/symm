package cvd

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Reading is the running executed-flow observation.
*/
type Reading struct {
	TradeCount   float64
	BuyCount     float64
	SellCount    float64
	BuyQty       float64
	SellQty      float64
	GrossQty     float64
	NetQty       float64
	BuyNotional  float64
	SellNotional float64
	GrossNotional float64
	NetNotional  float64
	MeanNotional float64
	CVD          float64
	CND          float64
	Epoch        float64
	SignedCount  float64
	SignedNet    float64
}

/*
Flow accumulates executions into running flow totals.
No structs, pure Value closure.
*/
type Flow types.Value[*Fill, Reading]

func NewFlow() Flow {
	var state Reading

	return func(fill *Fill) Reading {
		if fill == nil {
			return state
		}

		notional := fill.Price * fill.Qty
		state.TradeCount++
		state.GrossQty += fill.Qty
		state.GrossNotional += notional
		state.Epoch = float64(fill.At)

		if fill.Side == "buy" {
			state.BuyCount++
			state.BuyQty += fill.Qty
			state.BuyNotional += notional
		}

		if fill.Side == "sell" {
			state.SellCount++
			state.SellQty += fill.Qty
			state.SellNotional += notional
		}

		state.NetQty = state.BuyQty - state.SellQty
		state.NetNotional = state.BuyNotional - state.SellNotional
		state.CVD = state.NetQty
		state.CND = state.NetNotional

		if state.TradeCount > 0 {
			state.MeanNotional = state.GrossNotional / state.TradeCount
		}

		totalCount := state.BuyCount + state.SellCount
		if totalCount > 0 {
			state.SignedCount = (state.BuyCount - state.SellCount) / totalCount
		}

		if state.GrossNotional > 0 {
			state.SignedNet = state.NetNotional / state.GrossNotional
		}

		return state
	}
}
