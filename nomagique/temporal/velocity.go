package temporal

import (
	"time"

	"github.com/theapemachine/symm/nomagique/types"
)

// NanosPerSecond is the nanosecond/second unit conversion.
const NanosPerSecond = 1e9

/*
Velocity is the rate of change of its Source per unit of event time.

Source supplies the observed value and Clock the instant it was observed, so
the node retains the previous reading and its timestamp privately. A consumer
never tracks a previous value itself.

It returns 0 from Step, so it records inside a Split without disturbing the
parallel sum (Law of Sinks); the rate is read from Rate().

Degenerate behavior: an omitted Source or Clock cannot establish a rate. The
first observation has no predecessor, so its rate is 0.
*/
type Velocity struct {
	Source types.Node
	Clock  types.Node

	previous types.Number
	at       types.Number
	hasPrior bool
	rate     types.Number
}

func (velocity *Velocity) Step(x types.Number) types.Number {
	velocity.rate = 0

	if velocity.Source == nil || velocity.Clock == nil {
		return 0
	}

	value := velocity.Source.Step(x)
	at := velocity.Clock.Step(x)

	if velocity.hasPrior {
		if elapsed := at - velocity.at; elapsed > 0 {
			velocity.rate = (value - velocity.previous) / elapsed
		}
	}

	velocity.previous = value
	velocity.at = at
	velocity.hasPrior = true

	return 0
}

// Rate returns the most recent rate of change per unit of event time.
func (velocity *Velocity) Rate() types.Number { return velocity.rate }

// Value returns the most recent observed source reading.
func (velocity *Velocity) Value() types.Number { return velocity.previous }

// HasPrior reports whether a predecessor was observed.
func (velocity *Velocity) HasPrior() bool { return velocity.hasPrior }

/*
Clock holds the current event time as a slot, so a composition reads elapsed
time from the graph rather than the caller converting instants itself.
*/
type Clock struct {
	seconds types.Number
}

// Observe sets the clock to one instant, in seconds of event time.
func (clock *Clock) Observe(at time.Time) {
	clock.seconds = types.Number(at.UnixNano()) / NanosPerSecond
}

// Seconds returns the current event time in seconds.
func (clock *Clock) Seconds() types.Number { return clock.seconds }

func (clock *Clock) Step(types.Number) types.Number { return clock.seconds }

var (
	_ types.Node = (*Velocity)(nil)
	_ types.Node = (*Clock)(nil)
)
