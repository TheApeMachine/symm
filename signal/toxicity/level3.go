package toxicity

import (
	"fmt"

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
Input/output slot symbols for the book-touch disposition pipeline. The current
touch is loaded fresh each step; the previous touch is retained in committed
state and advanced by the trailing relay stages.
*/
var (
	symbolBidPrice   = nmtypes.MustIntern("toxicity/bid_price")
	symbolAskPrice   = nmtypes.MustIntern("toxicity/ask_price")
	symbolBidQty     = nmtypes.MustIntern("toxicity/bid_qty")
	symbolAskQty     = nmtypes.MustIntern("toxicity/ask_qty")
	symbolPrevBid    = nmtypes.MustIntern("toxicity/prev_bid_price")
	symbolPrevAsk    = nmtypes.MustIntern("toxicity/prev_ask_price")
	symbolPrevBidQty = nmtypes.MustIntern("toxicity/prev_bid_qty")
	symbolPrevAskQty = nmtypes.MustIntern("toxicity/prev_ask_qty")
	symbolBidLog     = nmtypes.MustIntern("toxicity/bid_log_change")
	symbolAskLog     = nmtypes.MustIntern("toxicity/ask_log_change")

	symbolAtSec      = nmtypes.MustIntern("toxicity/at_sec")
	symbolAtNsec     = nmtypes.MustIntern("toxicity/at_nsec")
	symbolPrevAtSec  = nmtypes.MustIntern("toxicity/prev_at_sec")
	symbolPrevAtNsec = nmtypes.MustIntern("toxicity/prev_at_nsec")
	symbolDeltaT     = nmtypes.MustIntern("toxicity/delta_t")
	symbolZero       = nmtypes.MustIntern("toxicity/zero")

	// symbolTouchComplete marks a frame whose bid and ask are BOTH known,
	// either from this message or from the symbol's committed state. A
	// one-sided message must still commit the side it carried, so
	// completeness gates the disposition pipeline instead of failing it.
	symbolTouchComplete = nmtypes.MustIntern("toxicity/touch_complete")

	// symbolTouchUncrossed marks a frame whose completed touch is a real book:
	// 0 < bid < ask. This feed is depth-limited and one-sided, so a fresh price
	// can transiently sit through the OTHER side's retained price. That frame
	// must still commit its fresh price, or the stale side is kept and every
	// later attribution brackets against a price nobody is quoting.
	symbolTouchUncrossed = nmtypes.MustIntern("toxicity/touch_uncrossed")

	// symbolSurrenderBid/Ask mark a side whose retained touch this message
	// withdrew without naming a replacement. The committed frame is merged
	// UNDER the input, so a surrendered side can only be cleared from inside
	// the pipeline, on the merged frame.
	symbolSurrenderBid = nmtypes.MustIntern("toxicity/surrender_bid")
	symbolSurrenderAsk = nmtypes.MustIntern("toxicity/surrender_ask")

	// The attributed previous touch is captured before the trailing relay
	// advances the retained previous state, so the projected provenance
	// reflects the touch being attributed rather than the fresh observation.
	symbolAttrPrevBid    = nmtypes.MustIntern("toxicity/attributed_prev_bid_price")
	symbolAttrPrevAsk    = nmtypes.MustIntern("toxicity/attributed_prev_ask_price")
	symbolAttrPrevBidQty = nmtypes.MustIntern("toxicity/attributed_prev_bid_qty")
	symbolAttrPrevAskQty = nmtypes.MustIntern("toxicity/attributed_prev_ask_qty")

	symbolBidQtyDiff       = nmtypes.MustIntern("toxicity/bid_qty_diff")
	symbolBidWithdrawalRaw = nmtypes.MustIntern("toxicity/bid_withdrawal_raw")
	symbolBidReplenishDiff = nmtypes.MustIntern("toxicity/bid_replenish_diff")
	symbolBidReplenishRaw  = nmtypes.MustIntern("toxicity/bid_replenish_raw")
	symbolBidUnchanged     = nmtypes.MustIntern("toxicity/bid_unchanged")
	symbolBidRetreat       = nmtypes.MustIntern("toxicity/bid_retreat")
	symbolBidWithdrawn     = nmtypes.MustIntern("toxicity/bid_withdrawn")
	symbolBidReplenished   = nmtypes.MustIntern("toxicity/bid_replenished")
	symbolBidRetreated     = nmtypes.MustIntern("toxicity/bid_retreated")

	symbolAskQtyDiff       = nmtypes.MustIntern("toxicity/ask_qty_diff")
	symbolAskWithdrawalRaw = nmtypes.MustIntern("toxicity/ask_withdrawal_raw")
	symbolAskReplenishDiff = nmtypes.MustIntern("toxicity/ask_replenish_diff")
	symbolAskReplenishRaw  = nmtypes.MustIntern("toxicity/ask_replenish_raw")
	symbolAskUnchanged     = nmtypes.MustIntern("toxicity/ask_unchanged")
	symbolAskRetreat       = nmtypes.MustIntern("toxicity/ask_retreat")
	symbolAskWithdrawn     = nmtypes.MustIntern("toxicity/ask_withdrawn")
	symbolAskReplenished   = nmtypes.MustIntern("toxicity/ask_replenished")
	symbolAskRetreated     = nmtypes.MustIntern("toxicity/ask_retreated")

	symbolBidReplenishFraction = nmtypes.MustIntern("toxicity/bid_replenish_fraction")
	symbolAskReplenishFraction = nmtypes.MustIntern("toxicity/ask_replenish_fraction")

	symbolBidWithdrawalRate = nmtypes.MustIntern("toxicity/bid_withdrawal_rate")
	symbolAskWithdrawalRate = nmtypes.MustIntern("toxicity/ask_withdrawal_rate")
	symbolBidReplenishRate  = nmtypes.MustIntern("toxicity/bid_replenish_rate")
	symbolAskReplenishRate  = nmtypes.MustIntern("toxicity/ask_replenish_rate")
	symbolBidRetreatRate    = nmtypes.MustIntern("toxicity/bid_retreat_rate")
	symbolAskRetreatRate    = nmtypes.MustIntern("toxicity/ask_retreat_rate")
)

/*
Namespaced per-side series for the fraction estimators. The empty prefix is the
legacy default; these prefixes let one frame carry independent withdrawal and
retreat fraction estimators per side.
*/
const (
	prefixWithdrawalBid = "withdrawal:bid"
	prefixWithdrawalAsk = "withdrawal:ask"
	prefixRetreatBid    = "retreat:bid"
	prefixRetreatAsk    = "retreat:ask"
)

var (
	withdrawalBidSample     = seriesFact(prefixWithdrawalBid, "sample")
	withdrawalBidSec        = seriesFact(prefixWithdrawalBid, "unix_sec")
	withdrawalBidNsec       = seriesFact(prefixWithdrawalBid, "unix_nsec")
	withdrawalBidBaseline   = seriesFact(prefixWithdrawalBid, "baseline/value")
	withdrawalBidDivergence = seriesFact(prefixWithdrawalBid, "z/residual")
	withdrawalBidZScore     = seriesFact(prefixWithdrawalBid, "z/value")
	withdrawalBidVelocity   = seriesFact(prefixWithdrawalBid, "velocity/delta")

	withdrawalAskSample     = seriesFact(prefixWithdrawalAsk, "sample")
	withdrawalAskSec        = seriesFact(prefixWithdrawalAsk, "unix_sec")
	withdrawalAskNsec       = seriesFact(prefixWithdrawalAsk, "unix_nsec")
	withdrawalAskBaseline   = seriesFact(prefixWithdrawalAsk, "baseline/value")
	withdrawalAskDivergence = seriesFact(prefixWithdrawalAsk, "z/residual")
	withdrawalAskZScore     = seriesFact(prefixWithdrawalAsk, "z/value")
	withdrawalAskVelocity   = seriesFact(prefixWithdrawalAsk, "velocity/delta")

	retreatBidSample   = seriesFact(prefixRetreatBid, "sample")
	retreatBidSec      = seriesFact(prefixRetreatBid, "unix_sec")
	retreatBidNsec     = seriesFact(prefixRetreatBid, "unix_nsec")
	retreatBidBaseline = seriesFact(prefixRetreatBid, "baseline/value")
	retreatBidZScore   = seriesFact(prefixRetreatBid, "z/value")

	retreatAskSample   = seriesFact(prefixRetreatAsk, "sample")
	retreatAskSec      = seriesFact(prefixRetreatAsk, "unix_sec")
	retreatAskNsec     = seriesFact(prefixRetreatAsk, "unix_nsec")
	retreatAskBaseline = seriesFact(prefixRetreatAsk, "baseline/value")
	retreatAskZScore   = seriesFact(prefixRetreatAsk, "z/value")
)

func seriesFact(prefix string, name string) nmtypes.Symbol {
	return nmtypes.MustIntern(temporal.JoinPrefix(prefix, name))
}

/*
Level3 is the book-touch market entity. It derives the current touch directly
from each Level3Data message's own visible bid/ask orders, attributes what
happened to the previously displayed touch between two comparable
observations, and projects one measurement. The entity owns exactly one
Number pipeline and one projector, both built in its constructor; it retains
no book but does retain each symbol's last observed touch, because Kraken
sends Level-3 as one-sided incremental updates.
*/
type Level3 struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewLevel3 constructs the Level3 entity: one Number pipeline for the touch
disposition computation and one projector that names the output slots.
*/
func NewLevel3() *Level3 {
	return &Level3{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			surrenderSides,
			// A one-sided message must still COMMIT, so the side it carried is
			// retained for the step that finally completes the touch. Number
			// only commits a frame whose Err is nil, so the disposition
			// pipeline is GATED on both sides being present rather than
			// guarded by a bare PositiveOrder that would fail the whole frame
			// and discard the very price that needs retaining.
			//
			// symbolTouchComplete is seeded by Step, which is the only place
			// that knows both what this message carried and what the symbol's
			// committed frame already holds.
			logic.If(
				nmtypes.Wire(
					nmtypes.Identity,
					nmtypes.In(symbolTouchUncrossed, logic.SymbolCondition),
					nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
				),
				nmtypes.Pipe(

					// Capture the touch being attributed before the trailing relay
					// advances the retained previous state.
					nmtypes.Relay(symbolPrevBid, symbolAttrPrevBid),
					nmtypes.Relay(symbolPrevAsk, symbolAttrPrevAsk),
					nmtypes.Relay(symbolPrevBidQty, symbolAttrPrevBidQty),
					nmtypes.Relay(symbolPrevAskQty, symbolAttrPrevAskQty),

					// Bracket duration between the previous and current observation.
					nmtypes.Wire(
						temporal.Duration,
						nmtypes.In(symbolAtSec, temporal.SymbolCurrentSec),
						nmtypes.In(symbolAtNsec, temporal.SymbolCurrentNsec),
						nmtypes.In(symbolPrevAtSec, temporal.SymbolPreviousSec),
						nmtypes.In(symbolPrevAtNsec, temporal.SymbolPreviousNsec),
						nmtypes.Out(temporal.SymbolDelta, symbolDeltaT),
					),

					// Touch price log change: log(P1/P0) per side.
					nmtypes.Wire(
						calculus.LogRatio,
						nmtypes.In(symbolBidPrice, calculus.SymbolCurrent),
						nmtypes.In(symbolPrevBid, calculus.SymbolPrevious),
						nmtypes.Out(calculus.PortResult, symbolBidLog),
					),
					nmtypes.Wire(
						calculus.LogRatio,
						nmtypes.In(symbolAskPrice, calculus.SymbolCurrent),
						nmtypes.In(symbolPrevAsk, calculus.SymbolPrevious),
						nmtypes.Out(calculus.PortResult, symbolAskLog),
					),

					// Bid disposition: net withdrawal and replenishment at an unchanged
					// price, and retreat when the bid moves away.
					nmtypes.Wire(
						calculus.Difference,
						nmtypes.In(symbolPrevBidQty, calculus.PortA),
						nmtypes.In(symbolBidQty, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolBidQtyDiff),
					),
					nmtypes.Wire(
						calculus.Positive,
						nmtypes.In(symbolBidQtyDiff, calculus.PortX),
						nmtypes.Out(calculus.PortResult, symbolBidWithdrawalRaw),
					),
					nmtypes.Wire(
						calculus.Difference,
						nmtypes.In(symbolBidQty, calculus.PortA),
						nmtypes.In(symbolPrevBidQty, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolBidReplenishDiff),
					),
					nmtypes.Wire(
						calculus.Positive,
						nmtypes.In(symbolBidReplenishDiff, calculus.PortX),
						nmtypes.Out(calculus.PortResult, symbolBidReplenishRaw),
					),
					nmtypes.Wire(
						logic.Equal,
						nmtypes.In(symbolBidPrice, calculus.PortA),
						nmtypes.In(symbolPrevBid, calculus.PortB),
						nmtypes.Out(logic.SymbolResult, symbolBidUnchanged),
					),
					nmtypes.Wire(
						logic.LessThan,
						nmtypes.In(symbolBidPrice, calculus.PortA),
						nmtypes.In(symbolPrevBid, calculus.PortB),
						nmtypes.Out(logic.SymbolResult, symbolBidRetreat),
					),
					nmtypes.Wire(
						logic.Gate,
						nmtypes.In(symbolBidUnchanged, logic.SymbolCondition),
						nmtypes.In(symbolBidWithdrawalRaw, logic.SymbolValue),
						nmtypes.Out(logic.SymbolResult, symbolBidWithdrawn),
					),
					nmtypes.Wire(
						logic.Gate,
						nmtypes.In(symbolBidUnchanged, logic.SymbolCondition),
						nmtypes.In(symbolBidReplenishRaw, logic.SymbolValue),
						nmtypes.Out(logic.SymbolResult, symbolBidReplenished),
					),
					nmtypes.Wire(
						logic.Gate,
						nmtypes.In(symbolBidRetreat, logic.SymbolCondition),
						nmtypes.In(symbolPrevBidQty, logic.SymbolValue),
						nmtypes.Out(logic.SymbolResult, symbolBidRetreated),
					),

					// Ask disposition mirrors the bid; ask retreat is a rising ask.
					nmtypes.Wire(
						calculus.Difference,
						nmtypes.In(symbolPrevAskQty, calculus.PortA),
						nmtypes.In(symbolAskQty, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolAskQtyDiff),
					),
					nmtypes.Wire(
						calculus.Positive,
						nmtypes.In(symbolAskQtyDiff, calculus.PortX),
						nmtypes.Out(calculus.PortResult, symbolAskWithdrawalRaw),
					),
					nmtypes.Wire(
						calculus.Difference,
						nmtypes.In(symbolAskQty, calculus.PortA),
						nmtypes.In(symbolPrevAskQty, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolAskReplenishDiff),
					),
					nmtypes.Wire(
						calculus.Positive,
						nmtypes.In(symbolAskReplenishDiff, calculus.PortX),
						nmtypes.Out(calculus.PortResult, symbolAskReplenishRaw),
					),
					nmtypes.Wire(
						logic.Equal,
						nmtypes.In(symbolAskPrice, calculus.PortA),
						nmtypes.In(symbolPrevAsk, calculus.PortB),
						nmtypes.Out(logic.SymbolResult, symbolAskUnchanged),
					),
					nmtypes.Wire(
						logic.GreaterThan,
						nmtypes.In(symbolAskPrice, calculus.PortA),
						nmtypes.In(symbolPrevAsk, calculus.PortB),
						nmtypes.Out(logic.SymbolResult, symbolAskRetreat),
					),
					nmtypes.Wire(
						logic.Gate,
						nmtypes.In(symbolAskUnchanged, logic.SymbolCondition),
						nmtypes.In(symbolAskWithdrawalRaw, logic.SymbolValue),
						nmtypes.Out(logic.SymbolResult, symbolAskWithdrawn),
					),
					nmtypes.Wire(
						logic.Gate,
						nmtypes.In(symbolAskUnchanged, logic.SymbolCondition),
						nmtypes.In(symbolAskReplenishRaw, logic.SymbolValue),
						nmtypes.Out(logic.SymbolResult, symbolAskReplenished),
					),
					nmtypes.Wire(
						logic.Gate,
						nmtypes.In(symbolAskRetreat, logic.SymbolCondition),
						nmtypes.In(symbolPrevAskQty, logic.SymbolValue),
						nmtypes.Out(logic.SymbolResult, symbolAskRetreated),
					),

					// Fractions over the attributed previous touch quantity.
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolBidWithdrawn, calculus.PortA),
						nmtypes.In(symbolAttrPrevBidQty, calculus.PortB),
						nmtypes.Out(calculus.PortResult, withdrawalBidSample),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolAskWithdrawn, calculus.PortA),
						nmtypes.In(symbolAttrPrevAskQty, calculus.PortB),
						nmtypes.Out(calculus.PortResult, withdrawalAskSample),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolBidReplenished, calculus.PortA),
						nmtypes.In(symbolAttrPrevBidQty, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolBidReplenishFraction),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolAskReplenished, calculus.PortA),
						nmtypes.In(symbolAttrPrevAskQty, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolAskReplenishFraction),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolBidRetreated, calculus.PortA),
						nmtypes.In(symbolAttrPrevBidQty, calculus.PortB),
						nmtypes.Out(calculus.PortResult, retreatBidSample),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolAskRetreated, calculus.PortA),
						nmtypes.In(symbolAttrPrevAskQty, calculus.PortB),
						nmtypes.Out(calculus.PortResult, retreatAskSample),
					),

					// Causal estimator chains for the withdrawal fraction, per side.
					temporal.Window(prefixWithdrawalBid),
					statistic.ZScore(prefixWithdrawalBid),
					statistic.Baseline(prefixWithdrawalBid),
					statistic.Velocity(prefixWithdrawalBid),
					temporal.Window(prefixWithdrawalAsk),
					statistic.ZScore(prefixWithdrawalAsk),
					statistic.Baseline(prefixWithdrawalAsk),
					statistic.Velocity(prefixWithdrawalAsk),

					// Causal estimator chains for the retreat fraction, per side.
					temporal.Window(prefixRetreatBid),
					statistic.ZScore(prefixRetreatBid),
					statistic.Baseline(prefixRetreatBid),
					temporal.Window(prefixRetreatAsk),
					statistic.ZScore(prefixRetreatAsk),
					statistic.Baseline(prefixRetreatAsk),

					// Rates over the bracket duration, emitted only when the bracket
					// is non-empty (a positive elapsed duration exists).
					nmtypes.Assign(symbolZero, 0),
					logic.If(
						nmtypes.Wire(
							logic.GreaterThan,
							nmtypes.In(symbolDeltaT, calculus.PortA),
							nmtypes.In(symbolZero, calculus.PortB),
							nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
						),
						nmtypes.Pipe(
							nmtypes.Wire(
								calculus.Rate,
								nmtypes.In(symbolBidWithdrawn, calculus.SymbolCount),
								nmtypes.In(symbolDeltaT, calculus.SymbolDuration),
								nmtypes.Out(calculus.SymbolRate, symbolBidWithdrawalRate),
							),
							nmtypes.Wire(
								calculus.Rate,
								nmtypes.In(symbolAskWithdrawn, calculus.SymbolCount),
								nmtypes.In(symbolDeltaT, calculus.SymbolDuration),
								nmtypes.Out(calculus.SymbolRate, symbolAskWithdrawalRate),
							),
							nmtypes.Wire(
								calculus.Rate,
								nmtypes.In(symbolBidReplenished, calculus.SymbolCount),
								nmtypes.In(symbolDeltaT, calculus.SymbolDuration),
								nmtypes.Out(calculus.SymbolRate, symbolBidReplenishRate),
							),
							nmtypes.Wire(
								calculus.Rate,
								nmtypes.In(symbolAskReplenished, calculus.SymbolCount),
								nmtypes.In(symbolDeltaT, calculus.SymbolDuration),
								nmtypes.Out(calculus.SymbolRate, symbolAskReplenishRate),
							),
							nmtypes.Wire(
								calculus.Rate,
								nmtypes.In(symbolBidRetreated, calculus.SymbolCount),
								nmtypes.In(symbolDeltaT, calculus.SymbolDuration),
								nmtypes.Out(calculus.SymbolRate, symbolBidRetreatRate),
							),
							nmtypes.Wire(
								calculus.Rate,
								nmtypes.In(symbolAskRetreated, calculus.SymbolCount),
								nmtypes.In(symbolDeltaT, calculus.SymbolDuration),
								nmtypes.Out(calculus.SymbolRate, symbolAskRetreatRate),
							),
						),
						nil,
					),

					// Advance the previous touch and clock to the current observation.
					nmtypes.Relay(symbolBidPrice, symbolPrevBid),
					nmtypes.Relay(symbolAskPrice, symbolPrevAsk),
					nmtypes.Relay(symbolBidQty, symbolPrevBidQty),
					nmtypes.Relay(symbolAskQty, symbolPrevAskQty),
					nmtypes.Relay(symbolAtSec, symbolPrevAtSec),
					nmtypes.Relay(symbolAtNsec, symbolPrevAtNsec),
				),
				nil,
			),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolBidPrice, Name: "best_price:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskPrice, Name: "best_price:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAttrPrevBid, Name: "previous_best_price:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAttrPrevAsk, Name: "previous_best_price:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidQty, Name: "touch_quantity:bid", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskQty, Name: "touch_quantity:ask", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAttrPrevBidQty, Name: "previous_touch_quantity:bid", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAttrPrevAskQty, Name: "previous_touch_quantity:ask", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidLog, Name: "touch_price_log_change:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskLog, Name: "touch_price_log_change:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAttrPrevBidQty, Name: "unfilled_residual_quantity:bid", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAttrPrevAskQty, Name: "unfilled_residual_quantity:ask", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidWithdrawn, Name: "net_withdrawn_quantity:bid", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskWithdrawn, Name: "net_withdrawn_quantity:ask", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidReplenished, Name: "net_replenished_quantity:bid", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskReplenished, Name: "net_replenished_quantity:ask", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidRetreated, Name: "retreated_quantity:bid", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskRetreated, Name: "retreated_quantity:ask", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: withdrawalBidSample, Name: "net_withdrawal_fraction:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: withdrawalAskSample, Name: "net_withdrawal_fraction:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidReplenishFraction, Name: "net_replenishment_fraction:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskReplenishFraction, Name: "net_replenishment_fraction:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: retreatBidSample, Name: "retreat_fraction:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: retreatAskSample, Name: "retreat_fraction:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: symbolBidWithdrawalRate, Name: "net_withdrawal_rate:bid", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskWithdrawalRate, Name: "net_withdrawal_rate:ask", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidReplenishRate, Name: "net_replenishment_rate:bid", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskReplenishRate, Name: "net_replenishment_rate:ask", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidRetreatRate, Name: "retreat_rate:bid", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskRetreatRate, Name: "retreat_rate:ask", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: withdrawalBidBaseline, Name: "withdrawal_fraction_baseline:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: withdrawalAskBaseline, Name: "withdrawal_fraction_baseline:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: withdrawalBidDivergence, Name: "withdrawal_fraction_divergence:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: withdrawalAskDivergence, Name: "withdrawal_fraction_divergence:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: withdrawalBidZScore, Name: "withdrawal_fraction_zscore:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: withdrawalAskZScore, Name: "withdrawal_fraction_zscore:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: withdrawalBidVelocity, Name: "withdrawal_fraction_velocity:bid", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: withdrawalAskVelocity, Name: "withdrawal_fraction_velocity:ask", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: retreatBidBaseline, Name: "retreat_fraction_baseline:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: retreatAskBaseline, Name: "retreat_fraction_baseline:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: retreatBidZScore, Name: "retreat_fraction_zscore:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: retreatAskZScore, Name: "retreat_fraction_zscore:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
	}
}

/*
bestTouch derives this message's own best bid and ask. It is a pure function of
the message: a side with no usable resting order returns zero, meaning "this
message said nothing about that side", NOT "that side is empty". Retention of
the untouched side is the Number pipeline's committed frame, not this
function's — a parallel Go-side copy of state the pipeline already holds per
symbol would be a second, unsynchronised source of truth.

A delete event reports the order being REMOVED from the book, so its price and
quantity describe vanished liquidity. Counting it as the touch would read
withdrawn size as resting size, so deletes are excluded here; the remaining
touch is whatever the message still shows resting.
*/
func (level3 *Level3) bestTouch(
	message kraken.Level3Data,
) (bidPrice, askPrice, bidQty, askQty float64) {
	for _, order := range message.Bids {
		if !order.Resting() {
			continue
		}

		if price := order.LimitPrice.Float64(); price > bidPrice {
			bidPrice = price
			bidQty = order.OrderQty.Float64()
		}
	}

	for _, order := range message.Asks {
		if !order.Resting() {
			continue
		}

		if price := order.LimitPrice.Float64(); askPrice == 0 || price < askPrice {
			askPrice = price
			askQty = order.OrderQty.Float64()
		}
	}

	return bidPrice, askPrice, bidQty, askQty
}

/*
Step derives this message's own touch, loads the touch facts, runs the Number
pipeline, and projects a measurement once the touch is complete.

Kraken sends Level-3 as one-sided incremental updates, so the side a message
did not carry must come from the symbol's committed frame. Only a side this
message actually showed is put into the input: Number merges the input OVER
the committed frame, so an omitted side keeps the value an earlier step
committed. A message that completes no touch yet still STEPS — committing the
side it carried — and simply reports nothing, because an incomplete touch is
the normal opening state of an incremental feed, not a failure.
*/
func (level3 *Level3) Step(message kraken.Level3Data) *data.Measurement[float64] {
	if level3 == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("toxicity: level3 entity missing for %s", message.Symbol)}
	}

	bidPrice, askPrice, bidQty, askQty := level3.bestTouch(message)
	symbol := message.Symbol
	at := message.Timestamp
	sec := float64(at.Unix())
	nsec := float64(at.Nanosecond())

	retainedBid, hasRetainedBid := 0.0, false
	retainedAsk, hasRetainedAsk := 0.0, false

	if prior, found := level3.number.Project(symbol); found {
		retainedBid, hasRetainedBid = prior.Get(symbolBidPrice)
		retainedAsk, hasRetainedAsk = prior.Get(symbolAskPrice)
	}

	input := nmtypes.Frame{}
	input.Put(symbolAtSec, sec)
	input.Put(symbolAtNsec, nsec)

	// A message announces orders; it does not restate the side. Kraken sends
	// Level-3 as one-sided incremental updates covering a depth window, so a
	// message adding a WORSE price than the one already resting says nothing
	// about the touch — the better order is still on the book. Taking this
	// message's price unconditionally would walk the retained touch away from
	// the best price and attribute against a level that is not the touch.
	//
	// An order AT the retained touch is accepted: a size change at the touch
	// price is the withdrawal/replenishment this signal exists to measure.
	//
	// A delete cannot be resolved this way: the next-best level lives in a
	// book this entity deliberately does not retain, so a message that
	// withdraws the retained touch surrenders the side rather than guessing.
	withdrewBid := withdrawsPrice(message.Bids, retainedBid, hasRetainedBid)
	withdrewAsk := withdrawsPrice(message.Asks, retainedAsk, hasRetainedAsk)

	if bidPrice == 0 && askPrice == 0 && !withdrewBid && !withdrewAsk {
		return nil
	}

	if bidPrice > 0 && (!hasRetainedBid || bidPrice >= retainedBid || withdrewBid) {
		input.Put(symbolBidPrice, bidPrice)
		input.Put(symbolBidQty, bidQty)
	}

	if askPrice > 0 && (!hasRetainedAsk || askPrice <= retainedAsk || withdrewAsk) {
		input.Put(symbolAskPrice, askPrice)
		input.Put(symbolAskQty, askQty)
	}

	loadSeriesClock(&input, sec, nsec)

	// Completeness is decided here because this is the only place that knows
	// both what the message carried and what the symbol's frame already holds.
	// A logic predicate cannot ask "is this slot present?" — it errors on an
	// absent input rather than reporting absence.
	hasBid, hasAsk := bidPrice > 0, askPrice > 0
	effectiveBid, effectiveAsk := bidPrice, askPrice
	effectiveBidQty, effectiveAskQty := bidQty, askQty
	committed, found := level3.number.Project(symbol)

	if found {
		if !hasBid {
			effectiveBid, hasBid = committed.Get(symbolBidPrice)
			effectiveBidQty, _ = committed.Get(symbolBidQty)
		}

		if !hasAsk {
			effectiveAsk, hasAsk = committed.Get(symbolAskPrice)
			effectiveAskQty, _ = committed.Get(symbolAskQty)
		}
	}

	// A withdrawn touch that this message did not replace leaves the side
	// genuinely unknown: the entity keeps no book, so it cannot name the next
	// level. Reporting the withdrawn price would publish liquidity that is
	// gone; the side is surrendered until the feed names a new touch.
	surrenderBid := withdrewBid && bidPrice == 0
	surrenderAsk := withdrewAsk && askPrice == 0

	if surrenderBid {
		hasBid = false
		effectiveBid, effectiveBidQty = 0, 0
	}

	if surrenderAsk {
		hasAsk = false
		effectiveAsk, effectiveAskQty = 0, 0
	}

	input.Put(symbolSurrenderBid, oneWhen(surrenderBid))
	input.Put(symbolSurrenderAsk, oneWhen(surrenderAsk))

	complete := 0.0

	if hasBid && hasAsk {
		complete = 1
	}

	input.Put(symbolTouchComplete, complete)

	// The touch is attributable only when it is a real book. The prices
	// compared here are the ones the pipeline will see: this message's own
	// price on a side it carried, the committed price on a side it did not.
	uncrossed := 0.0

	if complete == 1 && effectiveBid > 0 && effectiveBid < effectiveAsk {
		uncrossed = 1
	}

	input.Put(symbolTouchUncrossed, uncrossed)

	// The first UNCROSSED observation anchors the previous touch with itself: a
	// bracket of one observation has no movement to attribute. The anchor is
	// gated on the touch being a real book, not on the mere presence of a
	// previous bid: a one-sided opening commits its own side, so anchoring
	// then would leave the still-unseen side pinned at zero, and anchoring a
	// crossed touch would pin an inverted bracket that every later
	// attribution measures against.
	if uncrossed == 1 && (!found || !committed.Has(symbolPrevBid)) {
		input.Put(symbolPrevBid, effectiveBid)
		input.Put(symbolPrevAsk, effectiveAsk)
		input.Put(symbolPrevBidQty, effectiveBidQty)
		input.Put(symbolPrevAskQty, effectiveAskQty)
		input.Put(symbolPrevAtSec, sec)
		input.Put(symbolPrevAtNsec, nsec)
	}

	frame := level3.number.Step(symbol, input)

	// The step still ran, so this message's side is now committed and will
	// complete a later touch. It just has nothing to report yet: projecting
	// here would publish a measurement carrying one side alone, which reads
	// downstream as a real observation rather than a half-formed one.
	if uncrossed == 0 {
		return nil
	}

	return level3.projector.Project(symbol, "toxicity", at, at, frame)
}

/*
loadSeriesClock copies the observation clock into every per-side fraction
series so the namespaced estimators share one event-time coordinate.
*/
func loadSeriesClock(input *nmtypes.Frame, sec float64, nsec float64) {
	input.Put(withdrawalBidSec, sec)
	input.Put(withdrawalBidNsec, nsec)
	input.Put(withdrawalAskSec, sec)
	input.Put(withdrawalAskNsec, nsec)
	input.Put(retreatBidSec, sec)
	input.Put(retreatBidNsec, nsec)
	input.Put(retreatAskSec, sec)
	input.Put(retreatAskNsec, nsec)
}

func (level3 *Level3) Close() error { return nil }

/*
withdrawsPrice reports whether this message deletes the order resting at the
retained touch price. The next-best level is only knowable from a full book,
which this entity deliberately does not keep, so a withdrawn touch surrenders
the side and waits for the feed to name a new one.
*/
func withdrawsPrice(orders []kraken.Level3Order, retained float64, hasRetained bool) bool {
	if !hasRetained || retained <= 0 {
		return false
	}

	for _, order := range orders {
		if order.Event != "delete" || order.LimitPrice == nil {
			continue
		}

		if order.LimitPrice.Float64() == retained {
			return true
		}
	}

	return false
}

func oneWhen(condition bool) float64 {
	if condition {
		return 1
	}

	return 0
}

/*
surrenderSides clears a side whose retained touch was withdrawn without a
replacement. It runs first, on the frame Number has already merged, so every
later stage sees a side that is genuinely absent rather than one holding a
price that is no longer on the book.
*/
func surrenderSides(input *nmtypes.Frame) {
	if surrender, found := input.Get(symbolSurrenderBid); found && surrender != 0 {
		input.Delete(symbolBidPrice)
		input.Delete(symbolBidQty)
	}

	if surrender, found := input.Get(symbolSurrenderAsk); found && surrender != 0 {
		input.Delete(symbolAskPrice)
		input.Delete(symbolAskQty)
	}
}
