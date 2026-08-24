package temporal

import (
	"fmt"
	"math"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolCapacity = nmtypes.MustIntern("capacity")
	SymbolUnixSec  = nmtypes.MustIntern("unix_sec")
	SymbolUnixNsec = nmtypes.MustIntern("unix_nsec")
)

/*
Window returns the bounded retention ring primitive for one series prefix.
Retention is temporal plumbing: the window keeps whatever the feeder observes,
in arrival order, and every estimator downstream reads the same slots. One
frame can carry one independent window per prefix.

Capacity is not a magic number. It starts at one slot and doubles only when
the next observation would overflow the current ring, so a series earns more
memory by continuing to arrive — the count of observations is the only
governor. A caller may still seed an explicit capacity when it genuinely knows
one, but the default path stays data-driven and bounded by the engine's
fixed-sample ceiling.
*/
func Window(prefix string) nmtypes.Primitive {
	series := NewSeries(prefix)

	return func(input nmtypes.Frame) nmtypes.Frame {
		value, hasValue := input.Get(series.ValueSymbol)
		_, hasSec := input.Get(series.SecSymbol)
		nsec, hasNsec := input.Get(series.NsecSymbol)

		if !hasValue || !hasSec || !hasNsec {
			input.Err = fmt.Errorf(
				"temporal: window requires a value and event time",
			)

			return input
		}

		if nsec < 0 || nsec >= 1e9 {
			input.Err = fmt.Errorf(
				"temporal: window requires normalized nanoseconds",
			)

			return input
		}

		capacity, err := windowCapacity(series, input)

		if err != nil {
			input.Err = err

			return input
		}

		if capacity <= 0 || capacity > nmtypes.MaxSamples {
			input.Err = fmt.Errorf(
				"temporal: window capacity must be an integer from 1 through %d",
				nmtypes.MaxSamples,
			)

			return input
		}

		count := series.Count(input)
		head := series.Head(input)
		oldCapacity := capacity

		if current, found := input.Get(series.CapacitySymbol); found &&
			int(current) >= count && int(current) >= 1 {
			oldCapacity = int(current)
		}

		if count < 0 || count > oldCapacity || head < 0 || head >= oldCapacity {
			input.Err = fmt.Errorf(
				"temporal: window retained state is invalid for capacity %d",
				oldCapacity,
			)

			return input
		}

		// When the ring resizes while wrapped, its physical layout no longer
		// matches the new modulo base. Compact the retained samples back to
		// physical slots 0..count-1 in chronological order first, keeping the
		// newest samples when the ring shrinks.
		if oldCapacity != capacity && count > 0 {
			retained := count

			if retained > capacity {
				retained = capacity
			}

			values := make([]float64, retained)

			for index := 0; index < retained; index++ {
				source := (head + (count - retained) + index) % oldCapacity
				values[index], _ = series.SampleAt(&input, source)
			}

			for index := 0; index < retained; index++ {
				input.Put(series.SampleSymbol(index), values[index])
			}

			count = retained
			head = 0
		}

		slot := count

		if count >= capacity {
			slot = head
			head = (head + 1) % capacity
		} else {
			slot = (head + count) % capacity
			count++
		}

		input.Put(series.SampleSymbol(slot), value)
		input.Put(series.CountSymbol, float64(count))
		input.Put(series.HeadSymbol, float64(head))
		input.Put(series.ReadySymbol, 1)
		input.Put(series.CapacitySymbol, float64(capacity))

		return input
	}
}

/*
windowCapacity resolves how many slots the ring may retain. The Span control
channel wins when present, from the input first (a Configure-wired producer
emitted it this step) and then from state (a previous step's verdict that
Configure merged into state). Until a Span has been composed in, the window
bootstraps one slot and doubles only when the count reaches the current
capacity, bounded by the engine's sample ceiling.
*/
func windowCapacity(series Series, input nmtypes.Frame) (int, error) {
	capacityValue, found := input.Get(series.SpanSymbol)

	if found {
		if capacityValue <= 0 || capacityValue != math.Trunc(capacityValue) ||
			capacityValue > nmtypes.MaxSamples {
			return 0, fmt.Errorf(
				"temporal: span control channel must be an integer from 1 through %d",
				nmtypes.MaxSamples,
			)
		}

		return int(capacityValue), nil
	}

	current := series.Count(input)

	if current < 1 {
		return 1, nil
	}

	capacityValue, hasCapacity := input.Get(series.CapacitySymbol)

	if hasCapacity && current < int(capacityValue) {
		return int(capacityValue), nil
	}

	next := int(capacityValue + capacityValue)

	if next > nmtypes.MaxSamples {
		return nmtypes.MaxSamples, nil
	}

	return next, nil
}
