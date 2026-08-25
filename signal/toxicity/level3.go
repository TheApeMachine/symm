package toxicity

import (
	"fmt"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/kraken/websocket"
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
Level3 is the book-touch market entity. It reads the shared book from the
workspace pool, attributes what happened to the previously displayed touch
between two comparable book observations, and projects one measurement. The
entity owns exactly one Number pipeline and one projector, both built in its
constructor, plus the workspace it reads shared books from.
*/
type Level3 struct {
	workspace *runtime.Workspace
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewLevel3 constructs the Level3 entity: one Number pipeline for the touch
disposition computation and one projector that names the output slots.
*/
func NewLevel3(workspace *runtime.Workspace) *Level3 {
	return &Level3{
		workspace: workspace,
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			// A crossed, missing, or non-positive book is rejected here.
			logic.PositiveOrder(symbolBidPrice, symbolAskPrice),

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
Step receives one book observation for a symbol, loads the current touch facts,
runs the Number pipeline, and projects exactly one measurement. The shared book
is read through the workspace pool and type-asserted to *book.Book; a missing
book or a missing touch panics by design rather than being silently skipped.
*/
func (level3 *Level3) Step(symbol string, at time.Time) *data.Measurement[float64] {
	shared, found := level3.workspace.Shared("api", "")
	if !found {
		return &data.Measurement[float64]{Err: fmt.Errorf("toxicity: api missing for %s", symbol)}
	}

	api, ok := shared.(*websocket.API)
	if !ok || api == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("toxicity: api has unexpected type for %s", symbol)}
	}

	var hasQuote bool
	var bidPrice, askPrice, bidQty, askQty float64

	api.Book(symbol, func(currentBook *book.Book) {
		if currentBook == nil {
			return
		}
		bestBid := currentBook.BestBid()
		bestAsk := currentBook.BestAsk()

		if bestBid != nil && bestAsk != nil {
			bidPrice = bestBid.Price.Float64()
			askPrice = bestAsk.Price.Float64()
			bidQty = bestBid.Quantity.Float64()
			askQty = bestAsk.Quantity.Float64()
			hasQuote = true
		}
	})

	if !hasQuote {
		return &data.Measurement[float64]{Err: fmt.Errorf("toxicity: book touch missing for %s", symbol)}
	}
	sec := float64(at.Unix())
	nsec := float64(at.Nanosecond())

	input := nmtypes.Frame{}
	input.Put(symbolBidPrice, bidPrice)
	input.Put(symbolAskPrice, askPrice)
	input.Put(symbolBidQty, bidQty)
	input.Put(symbolAskQty, askQty)
	input.Put(symbolAtSec, sec)
	input.Put(symbolAtNsec, nsec)

	loadSeriesClock(&input, sec, nsec)

	committed, found := level3.number.Project(symbol)

	if !found || !committed.Has(symbolPrevBid) {
		// The first comparable observation anchors the previous touch with
		// itself: a bracket of one observation has no movement to attribute.
		input.Put(symbolPrevBid, bidPrice)
		input.Put(symbolPrevAsk, askPrice)
		input.Put(symbolPrevBidQty, bidQty)
		input.Put(symbolPrevAskQty, askQty)
		input.Put(symbolPrevAtSec, sec)
		input.Put(symbolPrevAtNsec, nsec)
	}

	return level3.projector.Project(
		symbol,
		"toxicity",
		at,
		at,
		level3.number.Step(symbol, input),
	)
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
