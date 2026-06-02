package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

/*
ProximityChainSupport counts tick windows where every category in the chain
appears within jitter ticks of each other, even when they never co-occur on
one snapshot.
*/
func (index *CoOccurrenceIndex) ProximityChainSupport(
	categories []perspectives.CategoryType,
	jitter int,
) int {
	required := uniqueCategories(categories)

	if len(required) == 0 || jitter < 0 {
		return 0
	}

	support := 0
	tickCount := len(index.tickSets)

	for tickIndex := range index.tickSets {
		windowStart := tickIndex - jitter
		windowEnd := tickIndex + jitter

		if windowStart < 0 {
			windowStart = 0
		}

		if windowEnd >= tickCount {
			windowEnd = tickCount - 1
		}

		union := make(map[perspectives.CategoryType]struct{})

		for windowIndex := windowStart; windowIndex <= windowEnd; windowIndex++ {
			for category := range index.tickSets[windowIndex] {
				union[category] = struct{}{}
			}
		}

		if categorySetContains(union, required) {
			support++
		}
	}

	return support
}

/*
ChainReachability reports hard co-occurrence, near-miss proximity, or absent.
*/
func (index *CoOccurrenceIndex) ChainReachability(
	categories []perspectives.CategoryType,
	jitter int,
) (hard bool, nearMiss bool) {
	if index == nil {
		return true, false
	}

	support := index.chainSupport(categories)

	if support >= index.minSupport {
		return true, false
	}

	if jitter <= 0 {
		return false, false
	}

	proximitySupport := index.ProximityChainSupport(categories, jitter)

	return false, proximitySupport > 0
}
