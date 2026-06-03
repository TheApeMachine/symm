package cooccurrence

import "math"

func deriveMinChainSupport(tickCount int) int {
	if tickCount <= 1 {
		return 1
	}

	support := int(math.Ceil(math.Sqrt(float64(tickCount))))

	if support < 2 {
		return 2
	}

	if support > tickCount {
		return tickCount
	}

	return support
}

func deriveNearMissTickJitter(tickCount int) int {
	if tickCount <= 1 {
		return 0
	}

	jitter := int(math.Ceil(math.Log2(float64(tickCount))))

	if jitter < 1 {
		return 1
	}

	return jitter
}
