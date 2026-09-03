package derivatives

import (
	"sync"
	"time"
)

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

func newCausalClock() causalClock {
	return causalClock{}
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
