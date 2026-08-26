package pumpdump

import (
	"time"

	book "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Input/output slot symbols for the volume-clock activity pipeline.
*/
var (
	symbolTradeNotional      = nmtypes.MustIntern("pumpdump/trade_notional")
	symbolBarQuantityTotal   = nmtypes.MustIntern("pumpdump/bar_quantity_total")
	symbolBarNotionalTotal   = nmtypes.MustIntern("pumpdump/bar_notional_total")
	symbolBarTradeCount      = nmtypes.MustIntern("pumpdump/bar_trade_count_total")
	symbolBarQuantity        = nmtypes.MustIntern("pumpdump/volume_bar_quantity")
	symbolBarNotional        = nmtypes.MustIntern("pumpdump/volume_bar_notional")
	symbolBarTradeCountSnap  = nmtypes.MustIntern("pumpdump/volume_bar_trade_count")
	symbolVolumeRate         = nmtypes.MustIntern("pumpdump/volume_rate")
	symbolNotionalRate       = nmtypes.MustIntern("pumpdump/notional_rate")
	symbolTradeRate          = nmtypes.MustIntern("pumpdump/trade_rate")
	symbolTradeInterval      = nmtypes.MustIntern("pumpdump/trade_interval_seconds")
	symbolHasMidpoint        = nmtypes.MustIntern("pumpdump/has_midpoint")
	symbolBarOpenMidpoint    = nmtypes.MustIntern("pumpdump/bar_open_midpoint")
	symbolMidpointLogReturn  = nmtypes.MustIntern("pumpdump/midpoint_log_return")
	symbolMidpointReturnRate = nmtypes.MustIntern("pumpdump/midpoint_return_rate")
	symbolNegMidpointReturn  = nmtypes.MustIntern("pumpdump/neg_midpoint_return")
	symbolPositiveReturn     = nmtypes.MustIntern("pumpdump/positive_midpoint_return")
	symbolNegativeReturn     = nmtypes.MustIntern("pumpdump/negative_midpoint_return")
	symbolLogNotionalRate    = nmtypes.MustIntern("pumpdump/log_notional_rate")
	symbolNotionalRateBase   = nmtypes.MustIntern("pumpdump/notional_rate_baseline")
	symbolNotionalRateRatio  = nmtypes.MustIntern("pumpdump/notional_rate_ratio")
)

/*
Trade is the volume-clock activity market entity. It owns exactly a Number
pipeline and a projector, both declared in its constructor, plus Step and Close.
*/
type Trade struct {
	workspace *runtime.Workspace
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTrade constructs the Trade entity: one Number pipeline for the quantity
clock, completed-bar rates, notional-rate context, and midpoint response, and
one projector that names the output slots.
*/
func NewTrade(workspace *runtime.Workspace) *Trade {
	return &Trade{
		workspace: workspace,
		number: nomagique.NewNumberWithInitial[string](
			func(key string) nmtypes.Frame {
				initial := nmtypes.Frame{}
				initial.Put(equation.SymbolClosed, 1)

				return initial
			},
			nmtypes.Pipe(
				nmtypes.Assign(symbolZero, 0),
				nmtypes.Assign(symbolOne, 1),
				// trade_interval_seconds: elapsed event time between trades.
				temporal.Observer("trade_interval", nmtypes.Quantity),
				logic.If(
					readyCondition(),
					nmtypes.Wire(
						temporal.Duration,
						nmtypes.In(nmtypes.EventTimeSec, temporal.SymbolCurrentSec),
						nmtypes.In(nmtypes.EventTimeNsec, temporal.SymbolCurrentNsec),
						nmtypes.In(prefixed("trade_interval", "temporal/observed_sec"), temporal.SymbolPreviousSec),
						nmtypes.In(prefixed("trade_interval", "temporal/observed_nsec"), temporal.SymbolPreviousNsec),
						nmtypes.Out(temporal.SymbolDelta, symbolTradeInterval),
					),
					nmtypes.Identity,
				),
				// Executable touch from the shared book, when present, and the
				// bar-opening midpoint latch: capture the midpoint when a new bar
				// opens, otherwise keep the retained opening midpoint.
				logic.If(
					hasMidpointCondition(),
					nmtypes.Pipe(
						nmtypes.Wire(
							calculus.Average,
							nmtypes.In(symbolBidPrice, calculus.PortA),
							nmtypes.In(symbolAskPrice, calculus.PortB),
							nmtypes.Out(calculus.PortResult, symbolMidpoint),
						),
						logic.If(
							openingCondition(),
							route(symbolMidpoint, symbolBarOpenMidpoint),
							nmtypes.Identity,
						),
					),
					nmtypes.Identity,
				),
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
				nmtypes.Wire(
					calculus.Accumulate,
					nmtypes.In(symbolOne, calculus.SymbolDelta),
					nmtypes.State(symbolBarTradeCount, calculus.SymbolTotal),
				),
				// Snapshot the running totals so they survive the bar reset.
				route(symbolBarQuantityTotal, symbolBarQuantity),
				route(symbolBarNotionalTotal, symbolBarNotional),
				route(symbolBarTradeCount, symbolBarTradeCountSnap),
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
						// notional_rate_velocity: first difference of the rate.
						velocityOver("notional_rate_velocity", symbolNotionalRate),
						// Positive notional rate enters a multiplicative log baseline.
						logic.If(
							greaterThanCondition(symbolNotionalRate),
							nmtypes.Pipe(
								nmtypes.Wire(
									calculus.Log,
									nmtypes.In(symbolNotionalRate, calculus.PortX),
									nmtypes.Out(calculus.PortResult, symbolLogNotionalRate),
								),
								// Causal notional-rate baseline/z-score, with the
								// departure and noise power mapped before the
								// baseline overwrites the series readiness.
								route(symbolLogNotionalRate, prefixed("notional_rate", "sample")),
								route(nmtypes.EventTimeSec, prefixed("notional_rate", "unix_sec")),
								route(nmtypes.EventTimeNsec, prefixed("notional_rate", "unix_nsec")),
								statistic.ZScore("notional_rate"),
								logic.If(
									seriesReadyCondition("notional_rate"),
									nmtypes.Pipe(
										route(prefixed("notional_rate", "z/residual"), symbolDivergence),
										nmtypes.Wire(
											calculus.Product,
											nmtypes.In(prefixed("notional_rate", "z/dispersion"), calculus.PortA),
											nmtypes.In(prefixed("notional_rate", "z/dispersion"), calculus.PortB),
											nmtypes.Out(calculus.PortResult, symbolNoiseVariance),
										),
									),
									nmtypes.Identity,
								),
								statistic.Baseline("notional_rate"),
								temporal.Window("notional_rate"),
								nmtypes.Wire(
									calculus.Exp,
									nmtypes.In(prefixed("notional_rate", "baseline/value"), calculus.PortX),
									nmtypes.Out(calculus.PortResult, symbolNotionalRateBase),
								),
								nmtypes.Wire(
									calculus.Quotient,
									nmtypes.In(symbolNotionalRate, calculus.PortA),
									nmtypes.In(symbolNotionalRateBase, calculus.PortB),
									nmtypes.Out(calculus.PortResult, symbolNotionalRateRatio),
								),
							),
							nmtypes.Identity,
						),
						// Midpoint response over the completed bar.
						logic.If(
							hasMidpointCondition(),
							logic.If(
								greaterThanCondition(symbolBarOpenMidpoint),
								nmtypes.Pipe(
									nmtypes.Wire(
										calculus.LogRatio,
										nmtypes.In(symbolMidpoint, calculus.SymbolCurrent),
										nmtypes.In(symbolBarOpenMidpoint, calculus.SymbolPrevious),
										nmtypes.Out(calculus.PortResult, symbolMidpointLogReturn),
									),
									nmtypes.Wire(
										calculus.Quotient,
										nmtypes.In(symbolMidpointLogReturn, calculus.PortA),
										nmtypes.In(calculus.SymbolDuration, calculus.PortB),
										nmtypes.Out(calculus.PortResult, symbolMidpointReturnRate),
									),
									nmtypes.Wire(
										calculus.Positive,
										nmtypes.In(symbolMidpointLogReturn, calculus.PortX),
										nmtypes.Out(calculus.PortResult, symbolPositiveReturn),
									),
									nmtypes.Wire(
										calculus.Negative,
										nmtypes.In(symbolMidpointLogReturn, calculus.PortX),
										nmtypes.Out(calculus.PortResult, symbolNegMidpointReturn),
									),
									nmtypes.Wire(
										calculus.Positive,
										nmtypes.In(symbolNegMidpointReturn, calculus.PortX),
										nmtypes.Out(calculus.PortResult, symbolNegativeReturn),
									),
									velocityOver("midpoint_return_velocity", symbolMidpointLogReturn),
									baselineZScore("midpoint_return", symbolMidpointLogReturn),
								),
								nmtypes.Identity,
							),
							nmtypes.Identity,
						),
						calculus.Clear(
							symbolBarQuantityTotal,
							symbolBarNotionalTotal,
							symbolBarTradeCount,
						),
					),
					nmtypes.Identity,
				),
			),
		),
		projector: data.NewProjector(
			data.Binding{From: nmtypes.AlphaPrice, Name: "trade_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: nmtypes.Quantity, Name: "trade_quantity", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolTradeNotional, Name: "trade_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolTradeInterval, Name: "trade_interval_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: equation.SymbolTarget, Name: "volume_bar_target_quantity", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBarQuantity, Name: "volume_bar_quantity", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBarNotional, Name: "volume_bar_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBarTradeCountSnap, Name: "volume_bar_trade_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: calculus.SymbolDuration, Name: "volume_bar_duration", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolVolumeRate, Name: "volume_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolNotionalRate, Name: "notional_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolTradeRate, Name: "trade_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: prefixed("notional_rate_velocity", "velocity/delta"), Name: "notional_rate_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolNotionalRateBase, Name: "notional_rate_baseline", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolNotionalRateRatio, Name: "notional_rate_ratio", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed("notional_rate", "z/residual"), Name: "notional_rate_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed("notional_rate", "z/value"), Name: "notional_rate_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBarOpenMidpoint, Name: "midpoint:from", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpoint, Name: "midpoint:at", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpoint, Name: "midpoint", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpointLogReturn, Name: "midpoint_log_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpointReturnRate, Name: "midpoint_return_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolPositiveReturn, Name: "positive_midpoint_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolNegativeReturn, Name: "negative_midpoint_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed("midpoint_return", "baseline/value"), Name: "midpoint_return_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed("midpoint_return", "z/residual"), Name: "midpoint_return_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed("midpoint_return", "z/value"), Name: "midpoint_return_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed("midpoint_return_velocity", "velocity/delta"), Name: "midpoint_return_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
		),
	}
}

/*
Step receives one trade data point, loads the tape facts and the shared book
midpoint, runs the Number pipeline, and projects exactly one Measurement. The
interval origin is read from the pipeline output to preserve the completed bar's
From time.
*/
func (trade *Trade) Step(point kraken.TradeData) *data.Measurement[float64] {
	input := nmtypes.Frame{}
	input.Put(nmtypes.Quantity, point.Qty)
	input.Put(nmtypes.AlphaPrice, point.Price.Float64())
	input.Put(nmtypes.EventTimeSec, float64(point.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(point.Timestamp.Nanosecond()))

	// The midpoint response family requires the executable touch from the shared
	// book; when the book is absent those metrics are simply undefined.
	var hasMidpoint bool
	inspectBook := func(resident *book.Book) {
		if resident == nil {
			return
		}
		bestBid := resident.BestBid()
		bestAsk := resident.BestAsk()

		if bestBid != nil && bestAsk != nil && bestBid.Price != nil && bestAsk.Price != nil {
			input.Put(symbolBidPrice, bestBid.Price.Float64())
			input.Put(symbolAskPrice, bestAsk.Price.Float64())
			hasMidpoint = true
		}
	}

	if shared, found := trade.workspace.Shared("api", ""); found && shared != nil {
		if api, ok := shared.(*websocket.API); ok && api != nil {
			api.Book(point.Symbol, inspectBook)
		}
	} else if sharedBook, found := trade.workspace.Shared("book", point.Symbol); found && sharedBook != nil {
		if currentBook, ok := sharedBook.(*book.Book); ok && currentBook != nil {
			inspectBook(currentBook)
		}
	}

	if hasMidpoint {
		input.Put(symbolHasMidpoint, 1)
	} else {
		input.Put(symbolHasMidpoint, 0)
	}

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

func hasMidpointCondition() nmtypes.Primitive {
	return nmtypes.Wire(
		nmtypes.Identity,
		nmtypes.In(symbolHasMidpoint, logic.SymbolCondition),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
}

func openingCondition() nmtypes.Primitive {
	return nmtypes.Wire(
		logic.Equal,
		nmtypes.In(equation.SymbolClosed, calculus.PortA),
		nmtypes.In(symbolOne, calculus.PortB),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
}
