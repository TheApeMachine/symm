package pumpdump

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Level3 is the authoritative executable-touch market entity. It owns exactly a
Number pipeline and a projector, both declared in its constructor, plus Step
and Close.

It retains no book and no touch of its own. Kraken sends Level-3 as one-sided
incremental updates, so a message carries a usable price on the side that
changed and nothing on the other — the previous touch on the untouched side
must survive into this step. That retention is exactly what Number already
provides: every key owns one committed Frame, and Step merges the incoming
frame OVER it, so a slot the message did not populate keeps its committed
value. Step therefore puts a side's price into the frame only when this
message actually carried one, and the pipeline's own state carries the other
side forward. A parallel Go-side map of last-seen touches would be a second,
unsynchronised copy of state the pipeline already holds per symbol.
*/
type Level3 struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewLevel3 constructs the Level3 entity: one Number pipeline for the executable
touch, and one projector that names the output slots.
*/
func NewLevel3() *Level3 {
	return &Level3{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			surrenderSides,
			// A one-sided message must still COMMIT, so its price is retained
			// in this symbol's frame for the step that finally completes the
			// touch. Number only commits a frame whose Err is nil, so the
			// touch metrics are gated on both sides being present rather than
			// guarded by a bare PositiveOrder that would fail the whole frame
			// (and discard the very price we need to retain).
			//
			// touchComplete is seeded by Step, which is where presence is
			// actually known: a logic predicate cannot read a slot that may
			// legitimately be absent without erroring on it.
			//
			// The same reasoning applies to a CROSSED touch. This feed is
			// depth-limited and one-sided, so a fresh price can transiently
			// sit through the opposite side's retained price. Failing the
			// frame there would discard the fresh price and keep the stale
			// one — measuring every later spread against a price nobody is
			// quoting — so the touch metrics are gated on being uncrossed
			// rather than guarded by a bare PositiveOrder.
			logic.If(
				nmtypes.Wire(
					nmtypes.Identity,
					nmtypes.In(symbolTouchUncrossed, logic.SymbolCondition),
					nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
				),
				nmtypes.Pipe(
					nmtypes.Wire(
						calculus.Average,
						nmtypes.In(symbolBidPrice, calculus.PortA),
						nmtypes.In(symbolAskPrice, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolMidpoint),
					),
					nmtypes.Wire(
						calculus.Difference,
						nmtypes.In(symbolAskPrice, calculus.PortA),
						nmtypes.In(symbolBidPrice, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolSpread),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolSpread, calculus.PortA),
						nmtypes.In(symbolMidpoint, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolRelativeSpread),
					),
				),
				nil,
			),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolBidPrice, Name: "best_bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskPrice, Name: "best_ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpoint, Name: "midpoint", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpread, Name: "spread", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelativeSpread, Name: "relative_spread", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
	}
}

/*
bestTouch derives this message's own best bid and ask. It is a pure function of
the message: a side with no usable resting order returns zero, meaning "this
message said nothing about that side", NOT "that side is empty". Retention of
the untouched side is the Number pipeline's committed frame, not this
function's.

A delete event reports the order being REMOVED from the book, so its price
describes liquidity that is gone. Treating it as the touch reports a price
nobody is quoting — and because a delete can be priced anywhere, including
through the opposite side's retained touch, it also manufactures a crossed
book out of a healthy one.
*/
func (level3 *Level3) bestTouch(
	message kraken.Level3Data,
) (bidPrice, askPrice float64) {
	for _, order := range message.Bids {
		if !order.Resting() {
			continue
		}

		if price := order.LimitPrice.Float64(); price > bidPrice {
			bidPrice = price
		}
	}

	for _, order := range message.Asks {
		if !order.Resting() {
			continue
		}

		if price := order.LimitPrice.Float64(); askPrice == 0 || price < askPrice {
			askPrice = price
		}
	}

	return bidPrice, askPrice
}

/*
Step loads this message's own touch prices and projects the result. Only a side
the message actually carried is put into the frame: Number merges the input
over the symbol's committed frame, so an omitted side keeps the price committed
by an earlier step. A symbol whose book has never shown both sides produces no
metrics at all rather than an error — an incomplete touch is the normal opening
state of an incremental feed, not a failure.
*/
func (level3 *Level3) Step(message kraken.Level3Data) *data.Measurement[float64] {
	bidPrice, askPrice := level3.bestTouch(message)

	input := nmtypes.Frame{}

	retainedBid, hasRetainedBid := 0.0, false
	retainedAsk, hasRetainedAsk := 0.0, false

	if prior, found := level3.number.Project(message.Symbol); found {
		retainedBid, hasRetainedBid = prior.Get(symbolBidPrice)
		retainedAsk, hasRetainedAsk = prior.Get(symbolAskPrice)
	}

	withdrewBid := withdrawsPrice(message.Bids, retainedBid, hasRetainedBid)
	withdrewAsk := withdrawsPrice(message.Asks, retainedAsk, hasRetainedAsk)

	// The coexisting price on each side is what the pipeline will actually
	// measure a spread against: this message's own price on a side it carried,
	// otherwise the committed (retained) price on a side it did not.
	effectiveBid, hasEffectiveBid := bidPrice, bidPrice > 0
	effectiveAsk, hasEffectiveAsk := askPrice, askPrice > 0

	if !hasEffectiveBid && hasRetainedBid {
		effectiveBid, hasEffectiveBid = retainedBid, true
	}

	if !hasEffectiveAsk && hasRetainedAsk {
		effectiveAsk, hasEffectiveAsk = retainedAsk, true
	}

	// A message announces orders; it does not restate the side. An order BEHIND
	// the retained touch on an uncrossed book says nothing about the touch —
	// the better order is still resting — so taking this message's price
	// unconditionally would walk the touch away from the best price and report
	// a spread wider than the one being quoted. An order AT the touch is a real
	// touch observation and is accepted.
	//
	// On a CROSSED retained book none of the two prices ever coexisted on the
	// wire (this feed is depth-limited and one-sided), so the retained price is
	// stale rather than the resting best. A fresh order that UNCROSSES the book
	// is the real, fresher touch and must displace the stale retained one;
	// refusing it would keep measuring every later spread against a price
	// nobody is quoting.
	bidRearmsTouch := hasRetainedBid && bidPrice > 0 && bidPrice >= retainedBid
	bidUncrosses := !hasEffectiveAsk || (retainedBid >= effectiveAsk && bidPrice < effectiveAsk)

	if bidPrice > 0 && (!hasRetainedBid || withdrewBid || bidRearmsTouch || bidUncrosses) {
		input.Put(symbolBidPrice, bidPrice)
	}

	askRearmsTouch := hasRetainedAsk && askPrice > 0 && askPrice <= retainedAsk
	askUncrosses := !hasEffectiveBid || (retainedAsk <= effectiveBid && askPrice > effectiveBid)

	if askPrice > 0 && (!hasRetainedAsk || withdrewAsk || askRearmsTouch || askUncrosses) {
		input.Put(symbolAskPrice, askPrice)
	}

	input.Put(symbolSurrenderBid, oneWhen(withdrewBid && bidPrice == 0))
	input.Put(symbolSurrenderAsk, oneWhen(withdrewAsk && askPrice == 0))

	if withdrewBid && bidPrice == 0 {
		hasEffectiveBid = false
	}

	if withdrewAsk && askPrice == 0 {
		hasEffectiveAsk = false
	}

	complete := 0.0

	if hasEffectiveBid && hasEffectiveAsk {
		complete = 1
	}

	input.Put(symbolTouchComplete, complete)

	// The touch is measurable only when it is a real book, i.e. when the two
	// prices the pipeline will coexist on the frame are not inverted.
	uncrossed := 0.0

	if complete == 1 && effectiveBid > 0 && effectiveBid < effectiveAsk {
		uncrossed = 1
	}

	input.Put(symbolTouchUncrossed, uncrossed)

	frame := level3.number.Step(message.Symbol, input)

	// The step still ran, so this message's price is now committed and will
	// complete a later touch. It just has nothing to report yet: projecting
	// here would publish a measurement carrying best_bid alone, which reads
	// downstream as a real observation rather than a half-formed one. A
	// crossed touch is the same situation — the fresh price is retained, and
	// the next message on the lagging side resolves it — so it reports
	// nothing rather than a spread taken across an inverted book.
	if uncrossed == 0 {
		return nil
	}

	return level3.projector.Project(
		message.Symbol,
		"pumpdump",
		message.Timestamp,
		message.Timestamp,
		frame,
	)
}

func (level3 *Level3) Close() error { return nil }

func oneWhen(condition bool) float64 {
	if condition {
		return 1
	}

	return 0
}

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

/*
surrenderSides clears a side whose retained touch was withdrawn without a
replacement. It runs first, on the frame Number has already merged, so every
later stage sees a side that is genuinely absent rather than one holding a
price that is no longer on the book.
*/
func surrenderSides(input *nmtypes.Frame) {
	if surrender, found := input.Get(symbolSurrenderBid); found && surrender != 0 {
		input.Delete(symbolBidPrice)
	}

	if surrender, found := input.Get(symbolSurrenderAsk); found && surrender != 0 {
		input.Delete(symbolAskPrice)
	}
}
