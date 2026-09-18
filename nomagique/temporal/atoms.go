package temporal

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewVelocity creates a stateful closure that tracks previous values and timestamps
to calculate the finite-difference rate of change (Velocity/Rate).
It expects an input array of [value, timestamp_seconds].
*/
func NewVelocity() types.Value[[2]float64, float64] {
	var prevValue, prevTime float64
	var hasPrior bool

	return func(in [2]float64) float64 {
		currentValue, currentTime := in[0], in[1]

		if !hasPrior {
			prevValue = currentValue
			prevTime = currentTime
			hasPrior = true
			return 0 // Rate is mathematically undefined on first observation
		}

		diff := currentValue - prevValue
		elapsed := currentTime - prevTime

		// Update state for next observation
		prevValue = currentValue
		prevTime = currentTime

		if elapsed > 0 {
			return diff / elapsed
		}
		return 0
	}
}

/*
NewLogReturns creates a stateful closure that tracks the previous value
and calculates the natural log of the ratio (Current/Previous).
*/
func NewLogReturns() types.Value[float64, float64] {
	var previous float64
	var initialized bool
	return func(in float64) float64 {
		if !initialized || previous <= 0 || in <= 0 {
			previous = in
			initialized = true
			return 0
		}
		ret := math.Log(in / previous)
		previous = in
		return ret
	}
}

/*
NewElapsed creates a stateful closure that tracks the previous timestamp (in nanoseconds)
and returns the elapsed time in seconds.
*/
func NewElapsed() types.Value[int64, float64] {
	var previous int64
	var initialized bool
	return func(in int64) float64 {
		if !initialized {
			previous = in
			initialized = true
			return 0
		}
		ret := float64(in-previous) / 1e9 // nanoseconds to seconds
		previous = in
		return ret
	}
}
