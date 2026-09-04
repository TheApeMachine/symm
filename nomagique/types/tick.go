package types

/*
Tick is the observation counter that makes a stateful node safe to place at
several points in one composition.

A measurement is rarely a chain. Gross notional is buy plus sell, net notional
is buy minus sell, and their ratio needs both: the accumulator holding buy
notional is therefore reached by three different paths in the same graph.
Evaluated naively it would advance three times and count one trade thrice.

Rather than route every shared quantity through a scratchpad — a shared array
of lanes indexed by constants, which is a register file wearing a stream
processor's clothes — a stateful node records which observation it last
advanced on. Reached again within the same observation, it returns what it
already computed. The graph may then be wired in any diamond it likes, and a
trade is counted exactly once no matter how many places depend on it.

Advance opens the next observation. Every composition steps it once, at the
boundary where market data enters.
*/
type Tick struct {
	current uint64
}

// Advance opens the next observation and returns its identity.
func (tick *Tick) Advance() uint64 {
	tick.current++

	return tick.current
}

// Current returns the observation now open.
func (tick *Tick) Current() uint64 { return tick.current }

/*
Guard is embedded by a stateful node to make it idempotent within one
observation.

Fresh reports whether this is the first time the node has been reached in the
current observation. A node that reports false must return its retained value
rather than advancing again.
*/
type Guard struct {
	tick *Tick
	seen uint64
}

/*
Bind attaches the observation counter this guard is measured against.
*/
func (guard *Guard) Bind(tick *Tick) { guard.tick = tick }

/*
Fresh reports whether the node should advance, and records that it has. It
returns false on every later reach within the same observation.

A node used on its own, outside any composition, has no counter to measure
against and no graph that could reach it twice, so it always advances. The
guard exists to make sharing safe, not to make a lone primitive inert.
*/
func (guard *Guard) Fresh() bool {
	if guard.tick == nil {
		return true
	}

	current := guard.tick.Current()

	if guard.seen == current {
		return false
	}

	guard.seen = current

	return true
}

// Started reports whether this guard has ever advanced.
func (guard *Guard) Started() bool { return guard.seen != 0 }
