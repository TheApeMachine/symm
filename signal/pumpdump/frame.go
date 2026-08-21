package pumpdump

import (
	"time"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func eventFrame(at time.Time) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(nmtypes.EventTimeSec, float64(at.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(at.Nanosecond()))

	return input
}

func sampleFrame(at time.Time, value float64) nomagique.Frame {
	input := eventFrame(at)
	input.Put(nomagique.SampleValue, value)

	return input
}

func absoluteSample(
	primitive nomagique.Primitive,
	input nomagique.Frame,
) (nomagique.Frame, error) {
	_, output, err := nomagique.Step(primitive, nomagique.Frame{}, input)

	return output, err
}

func polarizationFrame(
	change float64,
	normalized nomagique.Frame,
) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(equation.SymbolChange, change)
	mean, hasMean := normalized.Get(statistic.SymbolMean)
	ready, hasReady := normalized.Get(statistic.SymbolReady)

	if hasMean {
		input.Put(calculus.SymbolScale, mean)
	}

	if hasReady {
		input.Put(calculus.SymbolReady, ready)
	}

	return input
}

func observedFrom(output nomagique.Frame, fallback time.Time) time.Time {
	seconds, hasSeconds := output.Get(temporal.SymbolObservedSec)
	nanoseconds, hasNanoseconds := output.Get(temporal.SymbolObservedNsec)

	if !hasSeconds || !hasNanoseconds {
		return fallback
	}

	return time.Unix(int64(seconds), int64(nanoseconds)).UTC()
}
