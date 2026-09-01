package derivatives

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
symbolTimelineAdvanced is 1 when the event being stepped is the newest
observation on its symbol's causal timeline, and 0 when it arrived late. Every
pipeline stage that reads or advances the event clock is gated on it, so a late
event contributes its order-independent facts without being differenced,
windowed, or rate-divided as though it were the newest.
*/
var symbolTimelineAdvanced = nmtypes.MustIntern("derivatives/timeline_advanced")

/*
causalClock keeps one monotonic timeline per symbol, distinguishing a timestamp
the exchange actually sent from one the parser fabricated.

The observers downstream enforce "event time must not regress". Two very
different things can break that invariant, and they must not be treated alike:

A SYNTHETIC timestamp holds no truth about when the event happened -- the
payload carried no server time and [kraken.extractTimestamp] substituted the
local wall clock, a different time base that periodically reads as older than
the previous exchange-stamped event. There is no real time to protect, so the
timestamp is folded forward onto the timeline: the event ingests its facts and
only the meaningless clock label changes.

A REAL timestamp that regresses is a genuinely late event -- the futures feed
interleaves historical trade_snapshot batches with live trades. That time is a
fact about the world. Re-stamping it forward would fabricate a timeline and
silently corrupt every interval-denominated metric derived from it, so the
clock reports the regression instead and lets the caller account the event
without advancing the timeline.
*/
type causalClock struct {
	last sync.Map
}

/*
stamp folds one event onto the symbol's causal timeline.

It returns the event time to use and whether the timeline advanced. A false
`advanced` means the caller holds a genuinely late event: its facts are still
real and should still be accounted, but no interval origin, window, or
derivative may treat it as the newest observation.

A zero timestamp is never stamped: it would read as a regression after any real
observation, poisoning the first valid event.
*/
func (clock *causalClock) stamp(
	symbol string,
	timestamp time.Time,
	synthetic bool,
) (stamped time.Time, advanced bool) {
	if timestamp.IsZero() {
		return timestamp, true
	}

	loaded, _ := clock.last.Load(symbol)
	previous, _ := loaded.(time.Time)

	if timestamp.Before(previous) {
		// A fabricated timestamp carries no truth, so it is safe to pin it to
		// the timeline head. A real one is a fact and is reported as late.
		if synthetic {
			return previous, true
		}

		return timestamp, false
	}

	clock.last.Store(symbol, timestamp)

	return timestamp, true
}

/*
withoutStaleClockFacts runs `advance` when the event is the newest observation
on its symbol's timeline, and otherwise CLEARS the slots that stage would have
written.

Clearing is the whole point. A Frame is persistent state carried between steps,
so a gate that merely skips a stage leaves that stage's PREVIOUS value sitting
in the slot -- and the projector, which publishes every populated binding,
would then republish a stale number stamped with this event's identity. That is
worse than the wrong value it replaced: it is a wrong value wearing a fresh
timestamp. An absent metric is honest; a stale one is not.
*/
func withoutStaleClockFacts(
	advance nmtypes.Primitive,
	derived ...nmtypes.Symbol,
) nmtypes.Primitive {
	return logic.If(
		nmtypes.Wire(
			logic.GreaterThan,
			nmtypes.In(symbolTimelineAdvanced, calculus.PortA),
			nmtypes.In(symbolZero, calculus.PortB),
			nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
		),
		advance,
		func(frame *nmtypes.Frame) {
			for _, symbol := range derived {
				frame.Delete(symbol)
			}
		},
	)
}
