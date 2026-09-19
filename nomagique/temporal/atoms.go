package temporal

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

type Velocity types.Value[[2]float64, float64]

/*
NewVelocity creates a stateful closure that tracks previous values and timestamps
to calculate the finite-difference rate of change (Velocity/Rate).
It expects an input array of [value, timestamp_seconds].
*/
func NewVelocity() Velocity {
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

type LogReturns types.Value[float64, float64]

/*
NewLogReturns creates a stateful closure that tracks the previous value
and calculates the natural log of the ratio (Current/Previous).
*/
func NewLogReturns() LogReturns {
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

type Elapsed types.Value[int64, float64]

/*
NewElapsed creates a stateful closure that tracks the previous timestamp (in nanoseconds)
and returns the elapsed time in seconds.
*/
func NewElapsed() Elapsed {
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

type Transition types.Value[[]byte, []byte]

/*
NewTransition creates a stateful closure that emits sequential transitions across consecutive signatures.
On the initial observation, it retains previous without emitting.
On each subsequent observation, it emits (previous, current) encoded as "previous->current".
No structs, pure Value closure.
*/
func NewTransition() Transition {
	var previous []byte

	return func(current []byte) []byte {
		if len(current) == 0 {
			return nil
		}

		if len(previous) == 0 {
			previous = make([]byte, len(current))
			copy(previous, current)
			return nil
		}

		out := make([]byte, len(previous)+2+len(current))
		copy(out, previous)
		out[len(previous)] = '-'
		out[len(previous)+1] = '>'
		copy(out[len(previous)+2:], current)

		previous = make([]byte, len(current))
		copy(previous, current)
		return out
	}
}
