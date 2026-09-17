package cvd

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Reading is the running executed-flow observation.
*/
type Reading struct {
	TradeCount, BuyCount, SellCount                       float64
	BuyQty, SellQty, GrossQty, NetQty                     float64
	BuyNotional, SellNotional, GrossNotional, NetNotional float64
	MeanNotional, CVD, CND, Epoch                         float64
	SignedCount, SignedNet                                float64
}

/*
Flow accumulates aggressive executions into running flow totals.
*/
type Flow struct {
	*core.PrimitiveError

	out Reading
}

func NewFlow() *Flow {
	return &Flow{PrimitiveError: core.NewPrimitiveError()}
}

func (flow *Flow) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			fill := *(*Fill)(arriving)
			notional := fill.Price * fill.Qty
			flow.out.TradeCount++
			flow.out.GrossQty += fill.Qty
			flow.out.GrossNotional += notional

			if fill.Side == "buy" {
				flow.out.BuyCount++
				flow.out.BuyQty += fill.Qty
				flow.out.BuyNotional += notional
				flow.out.NetQty += fill.Qty
				flow.out.NetNotional += notional
				flow.out.CVD += fill.Qty
				flow.out.CND += notional
			}

			if fill.Side != "buy" {
				flow.out.SellCount++
				flow.out.SellQty += fill.Qty
				flow.out.SellNotional += notional
				flow.out.NetQty -= fill.Qty
				flow.out.NetNotional -= notional
				flow.out.CVD -= fill.Qty
				flow.out.CND -= notional
			}

			flow.out.MeanNotional = flow.out.GrossNotional / flow.out.TradeCount
			flow.out.SignedCount = (flow.out.BuyCount - flow.out.SellCount) / flow.out.TradeCount
			flow.out.SignedNet = flow.out.NetNotional / flow.out.GrossNotional
			flow.out.Epoch = float64(fill.At) / 1e9

			if !yield(unsafe.Pointer(&flow.out)) {
				return
			}
		}
	}
}
