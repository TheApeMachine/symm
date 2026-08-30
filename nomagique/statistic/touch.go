package statistic

import (
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Touch holds the running extremes of two opposing value streams and their
center. It is domain-neutral: the caller defines which side is "upper" and
which is "lower" purely by which slot it feeds, so no market vocabulary or
domain term ever enters this primitive.

Each side retains its own adaptive Path ring (span sized by that side's own
prior observations, never an imposed constant). The upper side reduces to its
maximum, the lower side to its minimum, and the center is the mean of those two
extremes. Slots are namespaced by prefix so one frame can hold independent
touches without collision.
*/
type touchSlots struct {
	upperMax types.Symbol
	lowerMin types.Symbol
	center   types.Symbol
	ready    types.Symbol
}

func newTouchSlots(prefix string) touchSlots {
	return touchSlots{
		upperMax: types.MustIntern(temporal.JoinPrefix(prefix, "touch/upper_extreme")),
		lowerMin: types.MustIntern(temporal.JoinPrefix(prefix, "touch/lower_extreme")),
		center:   types.MustIntern(temporal.JoinPrefix(prefix, "touch/center")),
		ready:    types.MustIntern(temporal.JoinPrefix(prefix, "touch/ready")),
	}
}

/*
Touch composes one center-of-two-streams reducer. upperSlot and lowerSlot name
the two values already present on the frame; secSlot and nsecSlot name the
shared event time. The primitive appends the upper value to the upper ring and
the lower value to the lower ring, reduces each ring to its own extreme, and
emits their mean as the center.
*/
func Touch(
	prefix string,
	upperSlot types.Symbol,
	lowerSlot types.Symbol,
	secSlot types.Symbol,
	nsecSlot types.Symbol,
) types.Primitive {
	upperSeries := temporal.NewSeries(prefix + "/upper")
	lowerSeries := temporal.NewSeries(prefix + "/lower")
	slots := newTouchSlots(prefix)

	upperPath := temporal.Path(prefix + "/upper")
	lowerPath := temporal.Path(prefix + "/lower")

	return func(input *types.Frame) {
		upper, hasUpper := input.Get(upperSlot)
		lower, hasLower := input.Get(lowerSlot)
		sec, hasSec := input.Get(secSlot)
		nsec, hasNsec := input.Get(nsecSlot)

		if !hasUpper || !hasLower || !hasSec || !hasNsec {
			input.Err = types.PrimitiveError(
				"statistic: touch requires upper, lower, and event time",
			)

			return
		}

		input.Put(upperSeries.ValueSymbol, upper)
		input.Put(upperSeries.SecSymbol, sec)
		input.Put(upperSeries.NsecSymbol, nsec)
		types.Step(upperPath, input)

		if input.Err != nil {
			return
		}

		input.Put(lowerSeries.ValueSymbol, lower)
		input.Put(lowerSeries.SecSymbol, sec)
		input.Put(lowerSeries.NsecSymbol, nsec)
		types.Step(lowerPath, input)

		if input.Err != nil {
			return
		}

		upperExtreme := trailExtreme(input, upperSeries, true)
		lowerExtreme := trailExtreme(input, lowerSeries, false)

		if upperExtreme <= 0 || lowerExtreme <= 0 {
			input.Put(slots.ready, 0)
			return
		}

		input.Put(slots.upperMax, upperExtreme)
		input.Put(slots.lowerMin, lowerExtreme)
		input.Put(slots.center, (upperExtreme+lowerExtreme)/2)
		input.Put(slots.ready, 1)
	}
}

/*
trailExtreme reduces one retained Path ring to its extreme: the maximum when
upper is true, the minimum otherwise. The ring's span is whatever the adaptive
Path chose; no window is imposed here.
*/
func trailExtreme(input *types.Frame, series temporal.Series, upper bool) float64 {
	count := series.CountPtr(input)

	if count <= 0 {
		return 0
	}

	extreme := 0.0
	initialized := false

	for index := 0; index < count; index++ {
		_, value, found := series.Sample(input, index)

		if !found {
			continue
		}

		if !initialized || (upper && value > extreme) || (!upper && value < extreme) {
			extreme = value
			initialized = true
		}
	}

	if !initialized {
		return 0
	}

	return extreme
}

/*
TouchCenter reads the center emitted by a Touch closure from a frame.
*/
func TouchCenter(frame *types.Frame, prefix string) (float64, bool) {
	slots := newTouchSlots(prefix)

	return frame.Get(slots.center)
}

/*
TouchReady reports whether a Touch closure has both sides populated.
*/
func TouchReady(frame *types.Frame, prefix string) bool {
	slots := newTouchSlots(prefix)
	ready, found := frame.Get(slots.ready)

	return found && ready != 0
}
