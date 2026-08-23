package pumpdump

import (
	"time"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func eventFrame(at time.Time) types.Frame {
	input := types.Frame{}
	input.Put(nmtypes.EventTimeSec, float64(at.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(at.Nanosecond()))

	return input
}

func sampleFrame(at time.Time, value float64) types.Frame {
	input := eventFrame(at)
	input.Put(nmtypes.SampleValue, value)

	return input
}

func absoluteSample(
	primitive nmtypes.Primitive,
	input types.Frame,
) (types.Frame, error) {
	_, output, err := nmtypes.Step(primitive, types.Frame{}, input)

	return output, err
}

func polarizationFrame(
	change float64,
	normalized types.Frame,
) types.Frame {
	input := types.Frame{}
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

func observedFrom(output types.Frame, fallback time.Time) time.Time {
	seconds, hasSeconds := output.Get(temporal.SymbolObservedSec)
	nanoseconds, hasNanoseconds := output.Get(temporal.SymbolObservedNsec)

	if !hasSeconds || !hasNanoseconds {
		return fallback
	}

	return time.Unix(int64(seconds), int64(nanoseconds)).UTC()
}
