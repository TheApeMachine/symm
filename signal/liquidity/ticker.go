package liquidity

import (
	"fmt"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Input/output slot symbols for the touch metric pipeline.
*/
var (
	symbolBidPrice         = nmtypes.MustIntern("liquidity/bid_price")
	symbolAskPrice         = nmtypes.MustIntern("liquidity/ask_price")
	symbolBidQty           = nmtypes.MustIntern("liquidity/bid_qty")
	symbolAskQty           = nmtypes.MustIntern("liquidity/ask_qty")
	symbolMidpoint         = nmtypes.MustIntern("liquidity/midpoint")
	symbolSpread           = nmtypes.MustIntern("liquidity/spread")
	symbolRelativeSpread   = nmtypes.MustIntern("liquidity/relative_spread")
	symbolBidNotional      = nmtypes.MustIntern("liquidity/touch_notional_bid")
	symbolAskNotional      = nmtypes.MustIntern("liquidity/touch_notional_ask")
	symbolTwoSidedNotional = nmtypes.MustIntern("liquidity/two_sided_notional")
	symbolSumNotional      = nmtypes.MustIntern("liquidity/sum_notional")
	symbolDiffNotional     = nmtypes.MustIntern("liquidity/diff_notional")
	symbolImbalance        = nmtypes.MustIntern("liquidity/touch_notional_imbalance")
)

/*
Ticker is the touch-snapshot market entity. It owns exactly a Number pipeline
and a projector, both declared in its constructor, plus Step and Close.
*/
type Ticker struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTicker constructs the Ticker entity: one Number pipeline for the touch
metric computation and one projector that names the output slots.
*/
func NewTicker() *Ticker {
	return &Ticker{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			// 0 < bid < ask: a crossed, missing, or non-positive book is rejected here.
			logic.PositiveOrder(symbolBidPrice, symbolAskPrice),
			// Bid notional: Db = bidPrice * bidQty
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolBidPrice, calculus.PortA),
				nmtypes.In(symbolBidQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBidNotional),
			),
			// Ask notional: Da = askPrice * askQty
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolAskPrice, calculus.PortA),
				nmtypes.In(symbolAskQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolAskNotional),
			),
			// Midpoint: (bid+ask)/2
			nmtypes.Wire(
				calculus.Average,
				nmtypes.In(symbolBidPrice, calculus.PortA),
				nmtypes.In(symbolAskPrice, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolMidpoint),
			),
			// Spread: ask-bid
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolAskPrice, calculus.PortA),
				nmtypes.In(symbolBidPrice, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolSpread),
			),
			// Relative spread: spread / midpoint
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolSpread, calculus.PortA),
				nmtypes.In(symbolMidpoint, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolRelativeSpread),
			),
			// Two-sided notional: min(Db, Da)
			nmtypes.Wire(
				calculus.Minimum,
				nmtypes.In(symbolBidNotional, calculus.PortA),
				nmtypes.In(symbolAskNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolTwoSidedNotional),
			),
			// Sum notional: Db + Da
			nmtypes.Wire(
				calculus.Sum,
				nmtypes.In(symbolBidNotional, calculus.PortA),
				nmtypes.In(symbolAskNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolSumNotional),
			),
			// Difference notional: Db - Da
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolBidNotional, calculus.PortA),
				nmtypes.In(symbolAskNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolDiffNotional),
			),
			// Imbalance: (Db - Da) / (Db + Da)
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolDiffNotional, calculus.PortA),
				nmtypes.In(symbolSumNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolImbalance),
			),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolBidPrice, Name: "best_bid_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskPrice, Name: "best_ask_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidQty, Name: "touch_quantity:bid", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskQty, Name: "touch_quantity:ask", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidNotional, Name: "touch_notional:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskNotional, Name: "touch_notional:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpoint, Name: "midpoint", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpread, Name: "spread", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelativeSpread, Name: "relative_spread", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolTwoSidedNotional, Name: "two_sided_touch_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolImbalance, Name: "touch_notional_imbalance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
	}
}

/*
Step receives one market data point, loads the touch facts, runs the Number
pipeline, and projects exactly one Measurement. Validation happens inside the
pipeline; any invalid input surfaces as a pipeline failure carried on the
Measurement's own Err field rather than a Go error return.
*/
func (ticker *Ticker) Step(trade kraken.TickerData) *data.Measurement[float64] {
	if trade.Bid == nil || trade.Ask == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: ticker requires bid and ask")}
	}

	input := nmtypes.Frame{}
	input.Put(symbolBidPrice, trade.Bid.Float64())
	input.Put(symbolAskPrice, trade.Ask.Float64())
	input.Put(symbolBidQty, trade.BidQty)
	input.Put(symbolAskQty, trade.AskQty)

	return ticker.projector.Project(
		trade.Symbol,
		"liquidity",
		trade.Timestamp,
		trade.Timestamp,
		ticker.number.Step(trade.Symbol, input),
	)
}

func (ticker *Ticker) Close() error { return nil }
