package cvd

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Reading is the running executed-flow observation for one symbol.
*/
type Reading struct {
	Symbol                                                  string
	TradeCount, BuyCount, SellCount                         float64
	BuyQty, SellQty, GrossQty, NetQty                       float64
	BuyNotional, SellNotional, GrossNotional, NetNotional   float64
	MeanNotional, CVD, CND, Epoch                           float64
	SignedCount, SignedNet                                  float64
}

/*
Flow accumulates aggressive executions into per-symbol flow totals.
*/
type Flow struct {
	*core.PrimitiveError

	paths map[string]*Reading
	out   Reading
}

func NewFlow() *Flow {
	return &Flow{PrimitiveError: core.NewPrimitiveError(), paths: make(map[string]*Reading)}
}

func (flow *Flow) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			fill := *(*Fill)(arriving)
			state := flow.paths[fill.Symbol]

			if state == nil {
				state = &Reading{Symbol: fill.Symbol}
				flow.paths[fill.Symbol] = state
			}

			notional := fill.Price * fill.Qty
			state.TradeCount++
			state.GrossQty += fill.Qty
			state.GrossNotional += notional

			if fill.Side == "buy" {
				state.BuyCount++
				state.BuyQty += fill.Qty
				state.BuyNotional += notional
				state.NetQty += fill.Qty
				state.NetNotional += notional
				state.CVD += fill.Qty
				state.CND += notional
			}

			if fill.Side != "buy" {
				state.SellCount++
				state.SellQty += fill.Qty
				state.SellNotional += notional
				state.NetQty -= fill.Qty
				state.NetNotional -= notional
				state.CVD -= fill.Qty
				state.CND -= notional
			}

			state.MeanNotional = state.GrossNotional / state.TradeCount
			state.SignedCount = (state.BuyCount - state.SellCount) / state.TradeCount
			state.SignedNet = state.NetNotional / state.GrossNotional
			state.Epoch = float64(fill.At) / 1e9
			flow.out = *state

			if !yield(unsafe.Pointer(&flow.out)) {
				return
			}
		}
	}
}
