package toxicity

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Trade-side fill attribution slots. The Trade entity matches each observed trade
against the current shared-book touch and accumulates fill statistics.
*/
var (
	symbolTradeQty        = nmtypes.MustIntern("toxicity/trade/qty")
	symbolTradePrice      = nmtypes.MustIntern("toxicity/trade/price")
	symbolSellFlag        = nmtypes.MustIntern("toxicity/trade/sell")
	symbolBuyFlag         = nmtypes.MustIntern("toxicity/trade/buy")
	symbolTradeBidPrice   = nmtypes.MustIntern("toxicity/trade/bid_price")
	symbolTradeAskPrice   = nmtypes.MustIntern("toxicity/trade/ask_price")
	symbolTradeBidQty     = nmtypes.MustIntern("toxicity/trade/bid_qty")
	symbolTradeAskQty     = nmtypes.MustIntern("toxicity/trade/ask_qty")
	symbolTradeAtSec      = nmtypes.MustIntern("toxicity/trade/at_sec")
	symbolTradeAtNsec     = nmtypes.MustIntern("toxicity/trade/at_nsec")
	symbolTradePrevAtSec  = nmtypes.MustIntern("toxicity/trade/prev_at_sec")
	symbolTradePrevAtNsec = nmtypes.MustIntern("toxicity/trade/prev_at_nsec")
	symbolTradeDeltaT     = nmtypes.MustIntern("toxicity/trade/delta_t")
	symbolTradeZero       = nmtypes.MustIntern("toxicity/trade/zero")

	symbolBracketQty = nmtypes.MustIntern("toxicity/trade/bracket_qty")

	symbolBidMatchFlag  = nmtypes.MustIntern("toxicity/trade/bid_match")
	symbolAskMatchFlag  = nmtypes.MustIntern("toxicity/trade/ask_match")
	symbolBidMatchedQty = nmtypes.MustIntern("toxicity/trade/bid_matched_qty")
	symbolAskMatchedQty = nmtypes.MustIntern("toxicity/trade/ask_matched_qty")
	symbolBidMatchedCum = nmtypes.MustIntern("toxicity/trade/bid_matched_cum")
	symbolAskMatchedCum = nmtypes.MustIntern("toxicity/trade/ask_matched_cum")
	symbolBidFillQty    = nmtypes.MustIntern("toxicity/trade/bid_fill_qty")
	symbolAskFillQty    = nmtypes.MustIntern("toxicity/trade/ask_fill_qty")
	symbolBidFillRate   = nmtypes.MustIntern("toxicity/trade/bid_fill_rate")
	symbolAskFillRate   = nmtypes.MustIntern("toxicity/trade/ask_fill_rate")
)

const (
	prefixFillBid = "fill:bid"
	prefixFillAsk = "fill:ask"
)

var (
	fillBidSample     = seriesFact(prefixFillBid, "sample")
	fillBidSec        = seriesFact(prefixFillBid, "unix_sec")
	fillBidNsec       = seriesFact(prefixFillBid, "unix_nsec")
	fillBidBaseline   = seriesFact(prefixFillBid, "baseline/value")
	fillBidDivergence = seriesFact(prefixFillBid, "z/residual")
	fillBidZScore     = seriesFact(prefixFillBid, "z/value")
	fillBidVelocity   = seriesFact(prefixFillBid, "velocity/delta")

	fillAskSample     = seriesFact(prefixFillAsk, "sample")
	fillAskSec        = seriesFact(prefixFillAsk, "unix_sec")
	fillAskNsec       = seriesFact(prefixFillAsk, "unix_nsec")
	fillAskBaseline   = seriesFact(prefixFillAsk, "baseline/value")
	fillAskDivergence = seriesFact(prefixFillAsk, "z/residual")
	fillAskZScore     = seriesFact(prefixFillAsk, "z/value")
	fillAskVelocity   = seriesFact(prefixFillAsk, "velocity/delta")
)

/*
Trade is the executed-flow market entity. It would match each trade against
the current book touch to attribute how much of the displayed touch the trade
tape accounts for, but has no access to book state; the fill-attribution
metrics are permanently undefined. It owns exactly one Number pipeline and one
projector.
*/
type Trade struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTrade constructs the Trade entity: one Number pipeline for fill attribution
and one projector that names the output slots.
*/
func NewTrade() *Trade {
	return &Trade{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			// Bracket trade quantity: cumulative executed quantity.
			nmtypes.Wire(
				calculus.Accumulate,
				nmtypes.In(symbolTradeQty, calculus.SymbolDelta),
				nmtypes.State(symbolBracketQty, calculus.SymbolTotal),
				nmtypes.Out(calculus.PortResult, symbolBracketQty),
			),

			// Bracket duration between consecutive trades.
			nmtypes.Wire(
				temporal.Duration,
				nmtypes.In(symbolTradeAtSec, temporal.SymbolCurrentSec),
				nmtypes.In(symbolTradeAtNsec, temporal.SymbolCurrentNsec),
				nmtypes.In(symbolTradePrevAtSec, temporal.SymbolPreviousSec),
				nmtypes.In(symbolTradePrevAtNsec, temporal.SymbolPreviousNsec),
				nmtypes.Out(temporal.SymbolDelta, symbolTradeDeltaT),
			),

			// Bid match: a sell executed at the bid touch.
			nmtypes.Wire(
				logic.Equal,
				nmtypes.In(symbolTradePrice, calculus.PortA),
				nmtypes.In(symbolTradeBidPrice, calculus.PortB),
				nmtypes.Out(logic.SymbolResult, symbolBidMatchFlag),
			),
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolSellFlag, calculus.PortA),
				nmtypes.In(symbolBidMatchFlag, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBidMatchFlag),
			),
			nmtypes.Wire(
				logic.Gate,
				nmtypes.In(symbolBidMatchFlag, logic.SymbolCondition),
				nmtypes.In(symbolTradeQty, logic.SymbolValue),
				nmtypes.Out(logic.SymbolResult, symbolBidMatchedQty),
			),
			nmtypes.Wire(
				calculus.Accumulate,
				nmtypes.In(symbolBidMatchedQty, calculus.SymbolDelta),
				nmtypes.State(symbolBidMatchedCum, calculus.SymbolTotal),
				nmtypes.Out(calculus.PortResult, symbolBidMatchedCum),
			),
			nmtypes.Wire(
				calculus.Minimum,
				nmtypes.In(symbolBidMatchedCum, calculus.PortA),
				nmtypes.In(symbolTradeBidQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBidFillQty),
			),

			// Ask match: a buy executed at the ask touch.
			nmtypes.Wire(
				logic.Equal,
				nmtypes.In(symbolTradePrice, calculus.PortA),
				nmtypes.In(symbolTradeAskPrice, calculus.PortB),
				nmtypes.Out(logic.SymbolResult, symbolAskMatchFlag),
			),
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolBuyFlag, calculus.PortA),
				nmtypes.In(symbolAskMatchFlag, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolAskMatchFlag),
			),
			nmtypes.Wire(
				logic.Gate,
				nmtypes.In(symbolAskMatchFlag, logic.SymbolCondition),
				nmtypes.In(symbolTradeQty, logic.SymbolValue),
				nmtypes.Out(logic.SymbolResult, symbolAskMatchedQty),
			),
			nmtypes.Wire(
				calculus.Accumulate,
				nmtypes.In(symbolAskMatchedQty, calculus.SymbolDelta),
				nmtypes.State(symbolAskMatchedCum, calculus.SymbolTotal),
				nmtypes.Out(calculus.PortResult, symbolAskMatchedCum),
			),
			nmtypes.Wire(
				calculus.Minimum,
				nmtypes.In(symbolAskMatchedCum, calculus.PortA),
				nmtypes.In(symbolTradeAskQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolAskFillQty),
			),

			// Fill fractions over the displayed touch quantity.
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolBidFillQty, calculus.PortA),
				nmtypes.In(symbolTradeBidQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, fillBidSample),
			),
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolAskFillQty, calculus.PortA),
				nmtypes.In(symbolTradeAskQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, fillAskSample),
			),

			// Causal estimator chains for the fill fraction, per side.
			temporal.Window(prefixFillBid),
			statistic.ZScore(prefixFillBid),
			statistic.Baseline(prefixFillBid),
			statistic.Velocity(prefixFillBid),
			// The bid-side fill fraction is this signal's headline metric, so its
			// estimator is the one whose departure and noise power become the
			// measurement's SNR.
			statistic.QualityFrom(prefixFillBid),
			temporal.Window(prefixFillAsk),
			statistic.ZScore(prefixFillAsk),
			statistic.Baseline(prefixFillAsk),
			statistic.Velocity(prefixFillAsk),

			// Fill rates over the trade spacing, only when spacing is positive.
			nmtypes.Assign(symbolTradeZero, 0),
			logic.If(
				nmtypes.Wire(
					logic.GreaterThan,
					nmtypes.In(symbolTradeDeltaT, calculus.PortA),
					nmtypes.In(symbolTradeZero, calculus.PortB),
					nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
				),
				nmtypes.Pipe(
					nmtypes.Wire(
						calculus.Rate,
						nmtypes.In(symbolBidFillQty, calculus.SymbolCount),
						nmtypes.In(symbolTradeDeltaT, calculus.SymbolDuration),
						nmtypes.Out(calculus.SymbolRate, symbolBidFillRate),
					),
					nmtypes.Wire(
						calculus.Rate,
						nmtypes.In(symbolAskFillQty, calculus.SymbolCount),
						nmtypes.In(symbolTradeDeltaT, calculus.SymbolDuration),
						nmtypes.Out(calculus.SymbolRate, symbolAskFillRate),
					),
				),
				nil,
			),

			// Advance the trade clock.
			nmtypes.Relay(symbolTradeAtSec, symbolTradePrevAtSec),
			nmtypes.Relay(symbolTradeAtNsec, symbolTradePrevAtNsec),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolBracketQty, Name: "bracket_trade_quantity", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidMatchedCum, Name: "matched_touch_trade_quantity:bid", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskMatchedCum, Name: "matched_touch_trade_quantity:ask", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidFillQty, Name: "touch_fill_quantity:bid", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskFillQty, Name: "touch_fill_quantity:ask", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: fillBidSample, Name: "touch_fill_fraction:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: fillAskSample, Name: "touch_fill_fraction:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidFillRate, Name: "touch_fill_rate:bid", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskFillRate, Name: "touch_fill_rate:ask", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: fillBidBaseline, Name: "fill_fraction_baseline:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: fillAskBaseline, Name: "fill_fraction_baseline:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: fillBidDivergence, Name: "fill_fraction_divergence:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: fillAskDivergence, Name: "fill_fraction_divergence:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: fillBidZScore, Name: "fill_fraction_zscore:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: fillAskZScore, Name: "fill_fraction_zscore:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: fillBidVelocity, Name: "fill_fraction_velocity:bid", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: fillAskVelocity, Name: "fill_fraction_velocity:ask", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
		),
	}
}

/*
Step matches one trade against the given touch (the Level3 side of this
signal's own last observation for the symbol — see Signal.Step) and projects
the fill attribution. A zero touch (nothing observed yet for this symbol)
yields no measurement.
*/
func (trade *Trade) Step(tick kraken.TradeData, bidPrice, askPrice, bidQty, askQty float64) *data.Measurement[float64] {
	if bidPrice == 0 || askPrice == 0 {
		return nil
	}

	sec := float64(tick.Timestamp.Unix())
	nsec := float64(tick.Timestamp.Nanosecond())

	sellFlag := 0.0
	buyFlag := 0.0

	if tick.Side == "sell" {
		sellFlag = 1.0
	}

	if tick.Side == "buy" {
		buyFlag = 1.0
	}

	input := nmtypes.Frame{}
	input.Put(symbolTradeQty, tick.Qty)
	input.Put(symbolTradePrice, tick.Price.Float64())
	input.Put(symbolSellFlag, sellFlag)
	input.Put(symbolBuyFlag, buyFlag)
	input.Put(symbolTradeBidPrice, bidPrice)
	input.Put(symbolTradeAskPrice, askPrice)
	input.Put(symbolTradeBidQty, bidQty)
	input.Put(symbolTradeAskQty, askQty)
	input.Put(symbolTradeAtSec, sec)
	input.Put(symbolTradeAtNsec, nsec)

	loadFillSeriesClock(&input, sec, nsec)

	committed, found := trade.number.Project(tick.Symbol)

	if found {
		prevSec, hasPrevSec := committed.Get(symbolTradeAtSec)
		prevNsec, hasPrevNsec := committed.Get(symbolTradeAtNsec)

		if hasPrevSec && hasPrevNsec &&
			(sec < prevSec || (sec == prevSec && nsec < prevNsec)) {
			return nil
		}
	}

	if !found || !committed.Has(symbolTradePrevAtSec) {
		input.Put(symbolTradePrevAtSec, sec)
		input.Put(symbolTradePrevAtNsec, nsec)
	}

	return trade.projector.Project(
		tick.Symbol,
		"toxicity",
		tick.Timestamp,
		tick.Timestamp,
		trade.number.Step(tick.Symbol, input),
	)
}

func loadFillSeriesClock(input *nmtypes.Frame, sec float64, nsec float64) {
	input.Put(fillBidSec, sec)
	input.Put(fillBidNsec, nsec)
	input.Put(fillAskSec, sec)
	input.Put(fillAskNsec, nsec)
}

func (trade *Trade) Close() error { return nil }
