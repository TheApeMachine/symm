package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolUnixSec  = nmtypes.MustIntern("unix_sec")
	SymbolUnixNsec = nmtypes.MustIntern("unix_nsec")

	SymbolBaselineValue        = nmtypes.MustIntern("baseline/value")
	SymbolBaselineEfficiency   = nmtypes.MustIntern("baseline/efficiency")
	SymbolBaselineWindow       = nmtypes.MustIntern("baseline/effective_window")
	SymbolBaselineLastSec      = nmtypes.MustIntern("baseline/last_sec")
	SymbolBaselineLastNsec     = nmtypes.MustIntern("baseline/last_nsec")
	SymbolBaselineSpan         = nmtypes.MustIntern("baseline/observed_span_sec")
	SymbolBaselineFastHalflife = nmtypes.MustIntern("baseline/fast_halflife_sec")
	SymbolBaselineSlowHalflife = nmtypes.MustIntern("baseline/slow_halflife_sec")

	// SymbolBaselineStability is the ring's relative dispersion stability, kept
	// in state so the window can read the previous step's verdict while it
	// decides whether to slide, grow, or shrink.
	SymbolBaselineStability = nmtypes.MustIntern("baseline/stability")
)

type baselineSlots struct {
	value        types.Symbol
	efficiency   types.Symbol
	window       types.Symbol
	lastSec      types.Symbol
	lastNsec     types.Symbol
	span         types.Symbol
	fastHalflife types.Symbol
	slowHalflife types.Symbol
	stability    types.Symbol
}

func newBaselineSlots(prefix string) baselineSlots {
	return baselineSlots{
		value:        types.MustIntern(temporal.JoinPrefix(prefix, "baseline/value")),
		efficiency:   types.MustIntern(temporal.JoinPrefix(prefix, "baseline/efficiency")),
		window:       types.MustIntern(temporal.JoinPrefix(prefix, "baseline/effective_window")),
		lastSec:      types.MustIntern(temporal.JoinPrefix(prefix, "baseline/last_sec")),
		lastNsec:     types.MustIntern(temporal.JoinPrefix(prefix, "baseline/last_nsec")),
		span:         types.MustIntern(temporal.JoinPrefix(prefix, "baseline/observed_span_sec")),
		fastHalflife: types.MustIntern(temporal.JoinPrefix(prefix, "baseline/fast_halflife_sec")),
		slowHalflife: types.MustIntern(temporal.JoinPrefix(prefix, "baseline/slow_halflife_sec")),
		stability:    types.MustIntern(temporal.JoinPrefix(prefix, "baseline/stability")),
	}
}

/*
Baseline returns the adaptive central estimate over one series' retained
samples, and the stability feedback that series' window needs to size itself.
The prefix namespaces every slot, so one frame can carry several independent
baselines. The empty prefix keeps the legacy generic slots.

The estimate is an event-time EMA: the first observation seeds the baseline
with itself (timescale "now", mean equal to the value), and every later
observation adapts with alpha = 1 - exp(-elapsed·ln2/halflife), where the
halflife is the data's own inter-arrival span unless a caller composes explicit
fast and slow horizons.

The feedback is relative dispersion: the largest residual of the retained ring
around its mean, scaled by the ring's own range, so stability is in [0, 1]
without an externally imposed threshold. A ring whose values are tightly
clustered around their mean is stable (near 1); a choppy ring is not (near 0).
The verdict is written back into state as the window-size modifier for the next
step: grow on a stability dip, slide while stability holds, and shrink toward
the actually used sample count when stability is perfect. Everything compared
against the data's own previous verdict — no magic numbers.
*/
func Baseline(prefix string) types.Primitive {
	series := temporal.NewSeries(prefix)
	slots := newBaselineSlots(prefix)

	return func(input types.Frame) types.Frame {
		value, hasValue := input.Get(series.ValueSymbol)
		sec, hasSec := input.Get(series.SecSymbol)
		nsec, hasNsec := input.Get(series.NsecSymbol)

		if !hasValue || !hasSec || !hasNsec {
			input.Err = fmt.Errorf(
				"statistic: baseline requires a value and event time",
			)

			return input
		}

		if nsec < 0 || nsec >= 1e9 {
			input.Err = fmt.Errorf(
				"statistic: baseline requires normalized nanoseconds",
			)

			return input
		}

		count := series.Count(input)

		previousSec, hasLastSec := input.Get(slots.lastSec)
		previousNsec, hasLastNsec := input.Get(slots.lastNsec)

		if hasLastSec && hasLastNsec {
			if elapsedSince(sec, nsec, previousSec, previousNsec) < 0 {
				input.Err = fmt.Errorf(
					"statistic: baseline event time must not regress",
				)

				return input
			}
		}

		_, hasBaseline := input.Get(slots.value)

		if count < 1 || !hasBaseline || !hasLastSec || !hasLastNsec {
			// The first observation is the baseline itself: timescale "now", one
			// unit of target window until more evidence exists.
			input.Put(slots.value, value)
			input.Put(slots.efficiency, 0)
			input.Put(slots.window, 1)
			input.Put(slots.span, 1)
			input.Put(slots.stability, 1)
			input.Put(slots.lastSec, sec)
			input.Put(slots.lastNsec, nsec)

			return baselineOutput(input, series, value, 1)
		}

		efficiency := windowEfficiency(series, input, count)
		elapsed := elapsedSince(sec, nsec, previousSec, previousNsec)
		halflife := baselineHalflife(elapsed, efficiency, input, slots)

		alpha := 1 - math.Exp(-elapsed*math.Ln2/halflife)

		baseline, _ := input.Get(slots.value)
		baseline += alpha * (value - baseline)

		stability := ringStability(series, input, count, baseline)
		previousStability, hasPreviousStability := input.Get(slots.stability)

		// A zero alpha (elapsed == 0) has no finite effective window; fall back
		// to the seeded single-unit window instead of storing infinity.
		window := 1.0

		if alpha > 0 {
			window = 2/alpha - 1
		}

		input.Put(slots.value, baseline)
		input.Put(slots.efficiency, efficiency)
		input.Put(slots.window, window)
		input.Put(slots.span, halflife)
		input.Put(slots.stability, stability)
		input.Put(slots.lastSec, sec)
		input.Put(slots.lastNsec, nsec)

		target := float64(count)

		if hasPreviousStability {
			target = windowModifier(series, input, count, stability, previousStability)
		}

		return baselineOutput(input, series, value, target)
	}
}

func baselineOutput(
	input types.Frame,
	series temporal.Series,
	value float64,
	target float64,
) types.Frame {
	input.Put(series.ValueSymbol, value)
	input.Put(series.ReadySymbol, 1)
	input.Put(series.SpanSymbol, target)

	return input
}

/*
windowEfficiency reports the retained ring's efficiency ratio: the absolute
net displacement between its oldest and newest samples over the path length
of every consecutive step, walked in arrival order. It measures how much of the
ring's motion was directional.
*/
func windowEfficiency(series temporal.Series, state types.Frame, count int) float64 {
	if count < 2 {
		return 0
	}

	capacity, _ := state.Get(series.CapacitySymbol)

	if capacity <= 0 {
		return 0
	}

	head := series.Head(state)
	slots := make([]float64, 0, count)

	for index := range count {
		sample, _ := series.SampleAt(&state, (head+index)%int(capacity))
		slots = append(slots, sample)
	}

	displacement := math.Abs(slots[len(slots)-1] - slots[0])
	path := 0.0

	for index := 1; index < len(slots); index++ {
		path += math.Abs(slots[index] - slots[index-1])
	}

	if path <= 0 {
		return 0
	}

	return displacement / path
}

/*
ringStability reports how tightly the retained ring clusters around the given
estimate, as a scale-free value in [0, 1]. The denominator is the ring's own
range; when the range collapses to zero the ring is perfectly stable. A choppy
ring is spread across its range and reports near zero, which is exactly the
signal the window needs to keep taking more inputs.
*/
func ringStability(
	series temporal.Series,
	state types.Frame,
	count int,
	estimate float64,
) float64 {
	if count < 1 {
		return 0
	}

	capacity, _ := state.Get(series.CapacitySymbol)

	if capacity <= 0 {
		return 1
	}

	head := series.Head(state)
	minimum := math.MaxFloat64
	maximum := -math.MaxFloat64
	largestResidual := 0.0

	for index := 0; index < count; index++ {
		sample, _ := series.SampleAt(&state, (head+index)%int(capacity))

		minimum = math.Min(minimum, sample)
		maximum = math.Max(maximum, sample)
		largestResidual = math.Max(largestResidual, math.Abs(sample-estimate))
	}

	rangeValue := maximum - minimum

	if rangeValue <= 0 {
		return 1
	}

	return clamp(1-largestResidual/rangeValue, 0, 1)
}

/*
windowModifier translates the stability verdict into the next capacity the
window should hold. Every comparison is against the data's own previous
verdict: a dip in stability asks the window to grow (take more inputs), a held
or improved verdict asks it to slide (keep its size), and perfect stability
asks it to shrink toward the samples it actually retains.
*/
func windowModifier(
	series temporal.Series,
	state types.Frame,
	count int,
	stability float64,
	previousStability float64,
) float64 {
	capacity, _ := state.Get(series.CapacitySymbol)

	if capacity <= 0 {
		capacity = float64(count)
	}

	// A single retained sample cannot prove stability: the ring has no range to
	// judge dispersion against. It must always grow to reach the minimum
	// evidence count before any shrink verdict is possible.
	if count < 2 {
		next := math.Max(2, capacity+1)

		if next > nmtypes.MaxSamples {
			return nmtypes.MaxSamples
		}

		return next
	}

	if stability < previousStability {
		// Continuous expansion scaled by the stability degradation
		expansionFactor := 1.0 + (previousStability - stability)
		next := math.Ceil(capacity * expansionFactor)

		if next <= capacity {
			next = capacity + 1
		}

		if next > nmtypes.MaxSamples {
			return nmtypes.MaxSamples
		}

		return next
	}

	if stability >= 1 {
		shrunk := math.Max(2, float64(count))

		return math.Min(capacity, shrunk)
	}

	return capacity
}

/*
baselineHalflife resolves the adaptation half-life from the data's own event
clock. Without explicit fast and slow horizons the half-life is the observed
inter-arrival span: one unit for the first observation ("now"), then "previous
now to now" for every later one, so the estimator's timescale is grown by the
data instead of imposed by a constant. Explicit horizons, when supplied, keep
the old efficiency interpolation for callers that genuinely own those bounds.
*/
func baselineHalflife(
	elapsed float64,
	efficiency float64,
	input types.Frame,
	slots baselineSlots,
) float64 {
	if fastHalflife, hasFast := input.Get(slots.fastHalflife); hasFast {
		if slowHalflife, hasSlow := input.Get(slots.slowHalflife); hasSlow &&
			fastHalflife > 0 && slowHalflife >= fastHalflife {
			return slowHalflife + efficiency*(fastHalflife-slowHalflife)
		}
	}

	span := math.Abs(elapsed)

	if span <= 0 {
		return 1
	}

	return span
}

func clamp(value float64, low float64, high float64) float64 {
	if value < low {
		return low
	}

	if value > high {
		return high
	}

	return value
}

func elapsedSince(
	sec float64,
	nsec float64,
	previousSec float64,
	previousNsec float64,
) float64 {
	return sec - previousSec + (nsec-previousNsec)*1e-9
}
