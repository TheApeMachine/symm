package toxicity

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/runtime"
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
)

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

			// Advance the previous touch to the current observation.
			nmtypes.Relay(symbolBidPrice, symbolPrevBid),
			nmtypes.Relay(symbolAskPrice, symbolPrevAsk),
			nmtypes.Relay(symbolBidQty, symbolPrevBidQty),
			nmtypes.Relay(symbolAskQty, symbolPrevAskQty),
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
	shared, _ := level3.workspace.Shared("book", symbol)
	currentBook := shared.(*book.Book)

	bidPrice := currentBook.BestBid().Price.Float64()
	askPrice := currentBook.BestAsk().Price.Float64()
	bidQty := currentBook.BestBid().Quantity.Float64()
	askQty := currentBook.BestAsk().Quantity.Float64()

	input := nmtypes.Frame{}
	input.Put(symbolBidPrice, bidPrice)
	input.Put(symbolAskPrice, askPrice)
	input.Put(symbolBidQty, bidQty)
	input.Put(symbolAskQty, askQty)

	committed, found := level3.number.Project(symbol)

	if !found || !committed.Has(symbolPrevBid) {
		// The first comparable observation anchors the previous touch with
		// itself: a bracket of one observation has no movement to attribute.
		input.Put(symbolPrevBid, bidPrice)
		input.Put(symbolPrevAsk, askPrice)
		input.Put(symbolPrevBidQty, bidQty)
		input.Put(symbolPrevAskQty, askQty)
	}

	return level3.projector.Project(
		symbol,
		"toxicity",
		at,
		at,
		level3.number.Step(symbol, input),
	)
}

func (level3 *Level3) Close() error { return nil }
