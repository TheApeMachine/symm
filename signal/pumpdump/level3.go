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
			logic.If(
				nmtypes.Wire(
					nmtypes.Identity,
					nmtypes.In(symbolTouchComplete, logic.SymbolCondition),
					nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
				),
				nmtypes.Pipe(
					// 0 < bid < ask: a crossed or non-positive book still
					// fails closed, but only once both sides are real.
					logic.PositiveOrder(symbolBidPrice, symbolAskPrice),
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
the message: a side with no usable order returns zero, meaning "this message
said nothing about that side", NOT "that side is empty". Retention of the
untouched side is the Number pipeline's committed frame, not this function's.
*/
func (level3 *Level3) bestTouch(
	message kraken.Level3Data,
) (bidPrice, askPrice float64) {
	for _, order := range message.Bids {
		if order.LimitPrice == nil {
			continue
		}

		if price := order.LimitPrice.Float64(); price > bidPrice {
			bidPrice = price
		}
	}

	for _, order := range message.Asks {
		if order.LimitPrice == nil {
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

	if bidPrice == 0 && askPrice == 0 {
		return nil
	}

	input := nmtypes.Frame{}

	if bidPrice > 0 {
		input.Put(symbolBidPrice, bidPrice)
	}

	if askPrice > 0 {
		input.Put(symbolAskPrice, askPrice)
	}

	// Completeness is decided here because this is the only place that knows
	// both what the message carried and what the symbol's frame already holds.
	// A logic predicate cannot ask "is this slot present?" — it errors on an
	// absent input rather than reporting absence.
	hasBid, hasAsk := bidPrice > 0, askPrice > 0

	if committed, found := level3.number.Project(message.Symbol); found {
		if !hasBid {
			_, hasBid = committed.Get(symbolBidPrice)
		}

		if !hasAsk {
			_, hasAsk = committed.Get(symbolAskPrice)
		}
	}

	complete := 0.0

	if hasBid && hasAsk {
		complete = 1
	}

	input.Put(symbolTouchComplete, complete)

	frame := level3.number.Step(message.Symbol, input)

	// The step still ran, so this message's price is now committed and will
	// complete a later touch. It just has nothing to report yet: projecting
	// here would publish a measurement carrying best_bid alone, which reads
	// downstream as a real observation rather than a half-formed one.
	if complete == 0 {
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
