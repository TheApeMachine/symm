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
type velocityState struct {
	previous types.Number
	at       types.Number
	hasPrior bool
	rate     types.Number
}

type Velocity struct {
	Source types.Node
	Clock  types.Node
	Key    func() string

	types.Guard

	previous types.Number
	at       types.Number
	hasPrior bool
	rate     types.Number

	states map[string]*velocityState
	active *velocityState
}

func (velocity *Velocity) key() string {
	if velocity.Key != nil {
		return velocity.Key()
	}

	return ""
}

func (velocity *Velocity) resolveState() *velocityState {
	activeKey := velocity.key()

	if activeKey != "" {
		if velocity.states == nil {
			velocity.states = make(map[string]*velocityState)
		}

		st, found := velocity.states[activeKey]

		if !found {
			st = &velocityState{}
			velocity.states[activeKey] = st
		}

		return st
	}

	return nil
}

func (velocity *Velocity) Step(x types.Number) types.Number {
	state := velocity.resolveState()
	velocity.active = state

	if state != nil {
		if !velocity.Fresh() {
			return 0
		}

		state.rate = 0

		if velocity.Source == nil || velocity.Clock == nil {
			return 0
		}

		value := velocity.Source.Step(x)
		at := velocity.Clock.Step(x)

		if state.hasPrior {
			if elapsed := at - state.at; elapsed > 0 {
				state.rate = (value - state.previous) / elapsed
			}
		}

		state.previous = value
		state.at = at
		state.hasPrior = true

		return 0
	}

	if !velocity.Fresh() {
		return 0
	}

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
func (velocity *Velocity) Rate() types.Number {
	if velocity.active != nil {
		return velocity.active.rate
	}

	return velocity.rate
}

/*
Readings publishes the rate of change. It is undefined on the first
observation, which has no predecessor to have changed from.
*/
func (velocity *Velocity) Readings() []types.Reading {
	return []types.Reading{{
		Label:     "velocity",
		Unit:      "per_second",
		Timescale: "per_second",
		Value:     velocity.Rate(),
		Defined:   velocity.HasPrior(),
	}}
}

// Value returns the most recent observed source reading.
func (velocity *Velocity) Value() types.Number {
	if velocity.active != nil {
		return velocity.active.previous
	}

	return velocity.previous
}

// HasPrior reports whether a predecessor was observed.
func (velocity *Velocity) HasPrior() bool {
	if velocity.active != nil {
		return velocity.active.hasPrior
	}

	return velocity.hasPrior
}

type clockState struct {
	seconds types.Number
	at      time.Time
}

/*
Clock holds the current event time as a slot, so a composition reads elapsed
time from the graph rather than the caller converting instants itself.
*/
type Clock struct {
	Key func() string

	seconds types.Number
	at      time.Time

	states map[string]*clockState
	active *clockState
}

func (clock *Clock) key() string {
	if clock.Key != nil {
		return clock.Key()
	}

	return ""
}

func (clock *Clock) resolveState() *clockState {
	activeKey := clock.key()

	if activeKey != "" {
		if clock.states == nil {
			clock.states = make(map[string]*clockState)
		}

		st, found := clock.states[activeKey]

		if !found {
			st = &clockState{}
			clock.states[activeKey] = st
		}

		return st
	}

	return nil
}

// Observe sets the clock to one instant, in seconds of event time.
func (clock *Clock) Observe(at time.Time) {
	state := clock.resolveState()
	clock.active = state

	secs := types.Number(at.UnixNano()) / NanosPerSecond

	if state != nil {
		state.at = at
		state.seconds = secs

		return
	}

	clock.at = at
	clock.seconds = secs
}

// Seconds returns the current event time in seconds.
func (clock *Clock) Seconds() types.Number {
	state := clock.resolveState()

	if state != nil {
		return state.seconds
	}

	return clock.seconds
}

// Time returns the current event time as time.Time.
func (clock *Clock) Time() time.Time {
	state := clock.resolveState()

	if state != nil {
		return state.at
	}

	return clock.at
}

func (clock *Clock) Step(types.Number) types.Number {
	return clock.Seconds()
}

var (
	_ types.Node = (*Velocity)(nil)
	_ types.Node = (*Clock)(nil)
)

// Slots exposes the nodes this velocity is composed of.
func (velocity *Velocity) Slots() []types.Node {
	return []types.Node{velocity.Source, velocity.Clock}
}
