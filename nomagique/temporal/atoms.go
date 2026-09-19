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
func NewVelocity(operands ...types.Value[any, [2]float64]) Velocity {
	var prevValue, prevTime float64
	var hasPrior bool

	return func(in [2]float64) float64 {
		input := in
		if len(operands) > 0 && operands[0] != nil {
			input = operands[0](in)
		}
		currentValue, currentTime := input[0], input[1]

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
func NewLogReturns(operands ...types.Float) LogReturns {
	var previous float64
	var initialized bool
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		if !initialized || previous <= 0 || val <= 0 {
			previous = val
			initialized = true
			return 0
		}
		ret := math.Log(val / previous)
		previous = val
		return ret
	}
}

type Elapsed types.Value[int64, float64]

/*
NewElapsed creates a stateful closure that tracks the previous timestamp (in nanoseconds)
and returns the elapsed time in seconds.
*/
func NewElapsed(operands ...types.Value[any, int64]) Elapsed {
	var previous int64
	var initialized bool
	return func(in int64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		if !initialized {
			previous = val
			initialized = true
			return 0
		}
		ret := float64(val-previous) / 1e9 // nanoseconds to seconds
		previous = val
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
func NewTransition(operands ...types.Bytes) Transition {
	var previous []byte

	return func(current []byte) []byte {
		cur := current
		if len(operands) > 0 && operands[0] != nil {
			cur = operands[0](current)
		}
		if len(cur) == 0 {
			return nil
		}

		if len(previous) == 0 {
			previous = make([]byte, len(cur))
			copy(previous, cur)
			return nil
		}

		out := make([]byte, len(previous)+2+len(cur))
		copy(out, previous)
		out[len(previous)] = '-'
		out[len(previous)+1] = '>'
		copy(out[len(previous)+2:], cur)

		previous = make([]byte, len(cur))
		copy(previous, cur)
		return out
	}
}
