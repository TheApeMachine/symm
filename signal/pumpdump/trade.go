package pumpdump

import (
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Input/output slot symbols for the volume-clock activity pipeline.
*/
var (
	symbolTradeNotional     = nmtypes.MustIntern("pumpdump/trade_notional")
	symbolBarQuantityTotal  = nmtypes.MustIntern("pumpdump/bar_quantity_total")
	symbolBarNotionalTotal  = nmtypes.MustIntern("pumpdump/bar_notional_total")
	symbolBarTradeCount     = nmtypes.MustIntern("pumpdump/bar_trade_count_total")
	symbolBarQuantity       = nmtypes.MustIntern("pumpdump/volume_bar_quantity")
	symbolBarNotional       = nmtypes.MustIntern("pumpdump/volume_bar_notional")
	symbolBarTradeCountSnap = nmtypes.MustIntern("pumpdump/volume_bar_trade_count")
	symbolVolumeRate        = nmtypes.MustIntern("pumpdump/volume_rate")
	symbolNotionalRate      = nmtypes.MustIntern("pumpdump/notional_rate")
	symbolTradeRate         = nmtypes.MustIntern("pumpdump/trade_rate")
)

/*
Trade is the volume-clock activity market entity. It owns exactly a Number
pipeline and a projector, both declared in its constructor, plus Step and Close.
*/
type Trade struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTrade constructs the Trade entity: one Number pipeline for the quantity
clock, completed-bar notional/count, and activity rates, and one projector
that names the output slots.
*/
func NewTrade() *Trade {
	return &Trade{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			// The volume clock sizes each bar from the symbol's own prior
			// quantity median and exposes the target, close flag, duration,
			// and the completed span's log price change.
			equation.Acceleration(),
			// Trade notional: n = price * qty
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(nmtypes.AlphaPrice, calculus.PortA),
				nmtypes.In(nmtypes.Quantity, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolTradeNotional),
			),
			// Accumulate quantity, notional, and trade count over the bar.
			nmtypes.Wire(
				calculus.Accumulate,
				nmtypes.In(nmtypes.Quantity, calculus.SymbolDelta),
				nmtypes.State(symbolBarQuantityTotal, calculus.SymbolTotal),
			),
			nmtypes.Wire(
				calculus.Accumulate,
				nmtypes.In(symbolTradeNotional, calculus.SymbolDelta),
				nmtypes.State(symbolBarNotionalTotal, calculus.SymbolTotal),
			),
			nmtypes.Assign(symbolOne, 1),
			nmtypes.Wire(
				calculus.Accumulate,
				nmtypes.In(symbolOne, calculus.SymbolDelta),
				nmtypes.State(symbolBarTradeCount, calculus.SymbolTotal),
			),
			// Snapshot the running totals so they survive the bar reset.
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(symbolBarQuantityTotal, calculus.PortX),
				nmtypes.Out(calculus.PortX, symbolBarQuantity),
			),
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(symbolBarNotionalTotal, calculus.PortX),
				nmtypes.Out(calculus.PortX, symbolBarNotional),
			),
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(symbolBarTradeCount, calculus.PortX),
				nmtypes.Out(calculus.PortX, symbolBarTradeCountSnap),
			),
			// Only a completed bar exposes throughput rates; the close trade is
			// included before the accumulators reset for the next bar.
			logic.If(
				closedCondition(),
				nmtypes.Pipe(
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolBarQuantityTotal, calculus.PortA),
						nmtypes.In(calculus.SymbolDuration, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolVolumeRate),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolBarNotionalTotal, calculus.PortA),
						nmtypes.In(calculus.SymbolDuration, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolNotionalRate),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolBarTradeCount, calculus.PortA),
						nmtypes.In(calculus.SymbolDuration, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolTradeRate),
					),
					calculus.Clear(
						symbolBarQuantityTotal,
						symbolBarNotionalTotal,
						symbolBarTradeCount,
					),
				),
				nmtypes.Identity,
			),
		)),
		projector: data.NewProjector(
			data.Binding{From: nmtypes.AlphaPrice, Name: "trade_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmtypes.Quantity, Name: "trade_quantity", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolTradeNotional, Name: "trade_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: equation.SymbolTarget, Name: "volume_bar_target_quantity", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBarQuantity, Name: "volume_bar_quantity", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBarNotional, Name: "volume_bar_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBarTradeCountSnap, Name: "volume_bar_trade_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: calculus.SymbolDuration, Name: "volume_bar_duration", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolVolumeRate, Name: "volume_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolNotionalRate, Name: "notional_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolTradeRate, Name: "trade_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
		),
	}
}

/*
Step receives one trade data point, loads the tape facts, runs the Number
pipeline, and projects exactly one Measurement. The interval origin is read
from the pipeline output to preserve the completed bar's From time.
*/
func (trade *Trade) Step(point kraken.TradeData) *data.Measurement[float64] {
	input := nmtypes.Frame{}
	input.Put(nmtypes.Quantity, point.Qty)
	input.Put(nmtypes.AlphaPrice, point.Price.Float64())
	input.Put(nmtypes.EventTimeSec, float64(point.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(point.Timestamp.Nanosecond()))

	output := trade.number.Step(point.Symbol, input)
	from := point.Timestamp

	if seconds, found := output.Get(temporal.SymbolObservedSec); found {
		if nanoseconds, found := output.Get(temporal.SymbolObservedNsec); found {
			from = time.Unix(int64(seconds), int64(nanoseconds)).UTC()
		}
	}

	return trade.projector.Project(point.Symbol, "pumpdump", point.Timestamp, from, output)
}

func (trade *Trade) Close() error { return nil }

func closedCondition() nmtypes.Primitive {
	return nmtypes.Wire(
		nmtypes.Identity,
		nmtypes.In(equation.SymbolClosed, logic.SymbolCondition),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
}
