package statistic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolVelocityDelta        = types.MustIntern("velocity/delta")
	SymbolVelocityAcceleration = types.MustIntern("velocity/acceleration")
	SymbolVelocityElapsed      = types.MustIntern("velocity/elapsed_sec")
	SymbolVelocityLastValue    = types.MustIntern("velocity/last_value")
	SymbolVelocityLastDelta    = types.MustIntern("velocity/last_delta")
	SymbolVelocityLastSec      = types.MustIntern("velocity/last_sec")
	SymbolVelocityLastNsec     = types.MustIntern("velocity/last_nsec")
)

type velocitySlots struct {
	delta        types.Symbol
	acceleration types.Symbol
	elapsed      types.Symbol
	lastValue    types.Symbol
	lastDelta    types.Symbol
	lastSec      types.Symbol
	lastNsec     types.Symbol
}

func newVelocitySlots(prefix string) velocitySlots {
	return velocitySlots{
		delta:        types.MustIntern(temporal.JoinPrefix(prefix, "velocity/delta")),
		acceleration: types.MustIntern(temporal.JoinPrefix(prefix, "velocity/acceleration")),
		elapsed:      types.MustIntern(temporal.JoinPrefix(prefix, "velocity/elapsed_sec")),
		lastValue:    types.MustIntern(temporal.JoinPrefix(prefix, "velocity/last_value")),
		lastDelta:    types.MustIntern(temporal.JoinPrefix(prefix, "velocity/last_delta")),
		lastSec:      types.MustIntern(temporal.JoinPrefix(prefix, "velocity/last_sec")),
		lastNsec:     types.MustIntern(temporal.JoinPrefix(prefix, "velocity/last_nsec")),
	}
}

/*
Velocity returns the first difference of one series' observed value on the
event clock, with the second difference carried alongside. The prefix
namespaces every slot; the empty prefix keeps the legacy generic slots.

A departure shows in the deltas long before it moves any baseline, so this
primitive never smooths: it reports the raw change and lets the composition
normalize it. The first observation seeds the differencer; the second produces
a delta; the third produces acceleration.
*/
func Velocity(prefix string) types.Primitive {
	series := temporal.NewSeries(prefix)
	slots := newVelocitySlots(prefix)

	return func(input types.Frame) types.Frame {
		value, hasValue := input.Get(series.ValueSymbol)
		sec, hasSec := input.Get(series.SecSymbol)
		nsec, hasNsec := input.Get(series.NsecSymbol)

		if !hasValue || !hasSec || !hasNsec {
			input.Err = fmt.Errorf(
				"statistic: velocity requires a value and event time",
			)

			return input
		}

		if nsec < 0 || nsec >= 1e9 {
			input.Err = fmt.Errorf(
				"statistic: velocity requires normalized nanoseconds",
			)

			return input
		}

		previousSec, hasLastSec := input.Get(slots.lastSec)
		previousNsec, hasLastNsec := input.Get(slots.lastNsec)

		if hasLastSec && hasLastNsec {
			if elapsedSince(sec, nsec, previousSec, previousNsec) < 0 {
				input.Err = fmt.Errorf(
					"statistic: velocity event time must not regress",
				)

				return input
			}
		}

		lastValue, hasLastValue := input.Get(slots.lastValue)
		lastDelta, hasLastDelta := input.Get(slots.lastDelta)

		input.Put(slots.lastValue, value)
		input.Put(slots.lastSec, sec)
		input.Put(slots.lastNsec, nsec)
		input.Put(series.ValueSymbol, value)
		input.Put(series.ReadySymbol, 0)

		if hasLastValue && hasLastSec && hasLastNsec {
			delta := value - lastValue
			elapsed := elapsedSince(sec, nsec, previousSec, previousNsec)

			input.Put(slots.lastDelta, delta)
			input.Put(slots.delta, delta)
			input.Put(slots.elapsed, elapsed)
			input.Put(series.ReadySymbol, 1)

			if hasLastDelta {
				input.Put(slots.acceleration, delta-lastDelta)
			}
		}

		return input
	}
}
