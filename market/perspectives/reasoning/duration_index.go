package reasoning

import (
	"time"

	"github.com/theapemachine/symm/market/perspectives/types"
)

type durationTailKey struct {
	subject  Subject
	category types.CategoryType
	within   int64
}

/*
durationIndex resolves a wall-clock lookback to a series index (0 = newest).
Results are cached per Reset so repeated predicates sharing the same Within pay
one linear scan per tick, then O(1) on subsequent reads.
*/
func (reason *WindowReason) durationIndex(
	times []time.Time,
	within time.Duration,
	subject Subject,
	category types.CategoryType,
) (int, bool) {
	if within <= 0 || len(times) == 0 || reason.now.IsZero() {
		return 0, false
	}

	if reason.durationTails == nil {
		reason.durationTails = make(map[durationTailKey]int, 4)
	}

	key := durationTailKey{
		subject:  subject,
		category: category,
		within:   within.Nanoseconds(),
	}

	if index, cached := reason.durationTails[key]; cached {
		if index < 0 || index >= len(times) {
			return 0, false
		}

		return index, true
	}

	cutoff := reason.now.Add(-within)
	thenIndex := len(times) - 1

	for index := len(times) - 1; index >= 0; index-- {
		if !times[index].After(cutoff) {
			thenIndex = index
			break
		}
	}

	reason.durationTails[key] = thenIndex

	return thenIndex, true
}

func (reason *WindowReason) resolveSignalIndex(
	category types.CategoryType,
	lookback Lookback,
) (int, bool) {
	series, ok := reason.signal[category]

	if !ok || len(series) == 0 {
		return 0, false
	}

	if lookback.Within > 0 {
		times := reason.signalTimes[category]

		return reason.durationIndex(times, lookback.Within, SubjectSignal, category)
	}

	if lookback.Ago < 0 || lookback.Ago >= len(series) {
		return 0, false
	}

	return lookback.Ago, true
}

func (reason *WindowReason) resolveScalarIndex(
	subject Subject,
	lookback Lookback,
) (int, bool) {
	var times []time.Time
	length := 0

	switch subject {
	case SubjectPrice:
		length = len(reason.price)
		times = reason.priceAt
	case SubjectVolume:
		length = len(reason.volume)
		times = reason.volumeAt
	case SubjectSpread:
		length = len(reason.spread)
		times = reason.spreadAt
	default:
		return 0, false
	}

	if length == 0 {
		return 0, false
	}

	if lookback.Within > 0 {
		return reason.durationIndex(times, lookback.Within, subject, types.CategoryTypeNone)
	}

	if lookback.Ago < 0 || lookback.Ago >= length {
		return 0, false
	}

	return lookback.Ago, true
}
