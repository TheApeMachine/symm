package derivatives

import (
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Input/output slot symbols for the derivatives liquidation-trade pipeline.
*/
var (
	symbolTradePrice       = nmtypes.MustIntern("derivatives/trade_price")
	symbolTradeQty         = nmtypes.MustIntern("derivatives/trade_quantity")
	symbolIsLiquidation    = nmtypes.MustIntern("derivatives/is_liquidation")
	symbolSideBuy          = nmtypes.MustIntern("derivatives/side_buy")
	symbolSideSell         = nmtypes.MustIntern("derivatives/side_sell")
	symbolTradeNotional    = nmtypes.MustIntern("derivatives/trade_notional")
	symbolLiqNotional      = nmtypes.MustIntern("derivatives/liq_notional")
	symbolBuyLiqDelta      = nmtypes.MustIntern("derivatives/buy_liq_delta")
	symbolSellLiqDelta     = nmtypes.MustIntern("derivatives/sell_liq_delta")
	symbolLiqBuyTotal      = nmtypes.MustIntern("derivatives/liq_buy_total")
	symbolLiqSellTotal     = nmtypes.MustIntern("derivatives/liq_sell_total")
	symbolGrossTradeTotal  = nmtypes.MustIntern("derivatives/gross_trade_total")
	symbolGrossLiq         = nmtypes.MustIntern("derivatives/gross_liquidation_notional")
	symbolNetLiq           = nmtypes.MustIntern("derivatives/net_liquidation_notional")
	symbolSignedFraction   = nmtypes.MustIntern("derivatives/liquidation_signed_fraction")
	symbolLiqRate          = nmtypes.MustIntern("derivatives/liquidation_notional_rate")
	symbolLiqShare         = nmtypes.MustIntern("derivatives/liquidation_share")
	symbolLiqShareVelocity = nmtypes.MustIntern("derivatives/liquidation_share_velocity")
)

/*
Trade is the liquidation-accounting market entity. It owns exactly a Number
pipeline and a projector, both declared in its constructor, plus Step and Close.
*/
type Trade struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTrade constructs the Trade entity: one Number pipeline for liquidation
notional accounting and one projector that names the output slots.
*/
func NewTrade() *Trade {
	return &Trade{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			nmtypes.Assign(symbolZero, 0),
			// Trade notional: n = price * qty
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolTradePrice, calculus.PortA),
				nmtypes.In(symbolTradeQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolTradeNotional),
			),
			// Retain notional for the interval's effective sample support.
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(symbolTradeNotional, calculus.PortX),
				nmtypes.Out(calculus.PortX, nmtypes.SampleValue),
			),
			temporal.Window(""),
			// Liquidation notional for this trade, zero unless it is a liquidation.
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolTradeNotional, calculus.PortA),
				nmtypes.In(symbolIsLiquidation, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolLiqNotional),
			),
			// Aggressor-side split of the liquidation notional.
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolLiqNotional, calculus.PortA),
				nmtypes.In(symbolSideBuy, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBuyLiqDelta),
			),
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolLiqNotional, calculus.PortA),
				nmtypes.In(symbolSideSell, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolSellLiqDelta),
			),
			// Interval accumulation: buy liquidations, sell liquidations, and all trades.
			nmtypes.Wire(
				calculus.Accumulate,
				nmtypes.In(symbolBuyLiqDelta, calculus.SymbolDelta),
				nmtypes.State(symbolLiqBuyTotal, calculus.SymbolTotal),
			),
			nmtypes.Wire(
				calculus.Accumulate,
				nmtypes.In(symbolSellLiqDelta, calculus.SymbolDelta),
				nmtypes.State(symbolLiqSellTotal, calculus.SymbolTotal),
			),
			nmtypes.Wire(
				calculus.Accumulate,
				nmtypes.In(symbolTradeNotional, calculus.SymbolDelta),
				nmtypes.State(symbolGrossTradeTotal, calculus.SymbolTotal),
			),
			// Gross and net liquidation notional over the retained interval.
			nmtypes.Wire(
				calculus.Sum,
				nmtypes.In(symbolLiqBuyTotal, calculus.PortA),
				nmtypes.In(symbolLiqSellTotal, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolGrossLiq),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolLiqBuyTotal, calculus.PortA),
				nmtypes.In(symbolLiqSellTotal, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolNetLiq),
			),
			// liquidation_signed_fraction is undefined when gross liquidation is zero.
			logic.If(
				greaterThanCondition(symbolGrossLiq),
				nmtypes.Wire(
					calculus.Quotient,
					nmtypes.In(symbolNetLiq, calculus.PortA),
					nmtypes.In(symbolGrossLiq, calculus.PortB),
					nmtypes.Out(calculus.PortResult, symbolSignedFraction),
				),
				nmtypes.Identity,
			),
			// Interval duration from the first retained trade.
			temporal.Since,
			// liquidation_notional_rate is undefined without positive interval duration.
			logic.If(
				greaterThanCondition(calculus.SymbolDuration),
				nmtypes.Wire(
					calculus.Quotient,
					nmtypes.In(symbolGrossLiq, calculus.PortA),
					nmtypes.In(calculus.SymbolDuration, calculus.PortB),
					nmtypes.Out(calculus.PortResult, symbolLiqRate),
				),
				nmtypes.Identity,
			),
			// liquidation_share is undefined when gross trade notional is zero.
			logic.If(
				greaterThanCondition(symbolGrossTradeTotal),
				nmtypes.Pipe(
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolGrossLiq, calculus.PortA),
						nmtypes.In(symbolGrossTradeTotal, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolLiqShare),
					),
					// liquidation_share_velocity: first difference of the share.
					velocityOver("liquidation_share", symbolLiqShare),
					nmtypes.Wire(
						nmtypes.Identity,
						nmtypes.In(prefixed("liquidation_share", "velocity/delta"), calculus.PortX),
						nmtypes.Out(calculus.PortX, symbolLiqShareVelocity),
					),
				),
				nmtypes.Identity,
			),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolLiqBuyTotal, Name: "liquidation_notional:buy", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLiqSellTotal, Name: "liquidation_notional:sell", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolGrossLiq, Name: "gross_liquidation_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolNetLiq, Name: "net_liquidation_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSignedFraction, Name: "liquidation_signed_fraction", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLiqRate, Name: "liquidation_notional_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolGrossTradeTotal, Name: "gross_derivative_trade_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLiqShare, Name: "liquidation_share", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLiqShareVelocity, Name: "liquidation_share_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
		),
	}
}

/*
Step receives one futures trade data point, loads the liquidation facts, runs
the Number pipeline, and projects exactly one Measurement. The interval origin
is read from the pipeline output to preserve the retained interval's From time.
*/
func (trade *Trade) Step(point kraken.FuturesTradeData) *data.Measurement[float64] {
	input := nmtypes.Frame{}
	input.Put(symbolTradePrice, point.Price.Float64())
	input.Put(symbolTradeQty, point.Qty)
	input.Put(symbolIsLiquidation, oneIf(point.Type == "liquidation"))
	input.Put(symbolSideBuy, oneIf(point.Side == "buy"))
	input.Put(symbolSideSell, oneIf(point.Side == "sell"))
	input.Put(nmtypes.EventTimeSec, float64(point.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(point.Timestamp.Nanosecond()))

	output := trade.number.Step(point.Symbol, input)
	from := point.Timestamp

	if seconds, found := output.Get(temporal.SymbolObservedSec); found {
		if nanoseconds, found := output.Get(temporal.SymbolObservedNsec); found {
			from = time.Unix(int64(seconds), int64(nanoseconds)).UTC()
		}
	}

	return trade.projector.Project(point.Symbol, "derivatives", point.Timestamp, from, output)
}

func (trade *Trade) Close() error { return nil }

func oneIf(condition bool) float64 {
	if condition {
		return 1
	}

	return 0
}
