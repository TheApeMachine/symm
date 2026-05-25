package causal

import (
	"math"
	"time"

	"github.com/theapemachine/symm/numeric"
)

/*
opportunityRunway estimates how long excess velocity persists versus history.
*/
func opportunityRunway(samples []causalSample, lastElapsed time.Duration) time.Duration {
	if lastElapsed <= 0 || len(samples) == 0 {
		return 0
	}

	current := samples[len(samples)-1]
	speed := math.Abs(current.priceVelocity)
	typical := numeric.MedianAbsolute(extract(samples, func(sample causalSample) float64 {
		return sample.priceVelocity
	}))

	if speed <= 0 {
		return lastElapsed
	}

	if typical <= 0 {
		return lastElapsed
	}

	factor := speed / typical

	if factor <= 0 {
		return lastElapsed
	}

	return time.Duration(float64(lastElapsed) / factor)
}
