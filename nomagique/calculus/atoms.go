package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

type Tanh types.Value[float64, float64]
func NewTanh() Tanh { return math.Tanh }

type Absolute types.Value[float64, float64]
func NewAbsolute() Absolute { return math.Abs }

type Atanh types.Value[float64, float64]
func NewAtanh() Atanh { return math.Atanh }

type Erfc types.Value[float64, float64]
func NewErfc() Erfc { return math.Erfc }

type Exp types.Value[float64, float64]
func NewExp() Exp { return math.Exp }

type Floor types.Value[float64, float64]
func NewFloor() Floor { return math.Floor }

type Log types.Value[float64, float64]
func NewLog() Log { return math.Log }

type Sqrt types.Value[float64, float64]
func NewSqrt() Sqrt { return math.Sqrt }

type Reciprocal types.Value[float64, float64]
func NewReciprocal() Reciprocal {
	return func(in float64) float64 {
		if in == 0 {
			return 0
		}
		return core.Unit / in
	}
}

type Square types.Value[float64, float64]
func NewSquare() Square {
	return func(in float64) float64 {
		return in * in
	}
}

type Sign types.Value[float64, float64]
func NewSign() Sign {
	return func(in float64) float64 {
		if in < 0 {
			return -1
		} else if in > 0 {
			return 1
		}
		return 0
	}
}

type Negate types.Value[float64, float64]
func NewNegate() Negate {
	return func(in float64) float64 {
		return -in
	}
}

type Maximum types.Value[[2]float64, float64]
func NewMaximum() Maximum {
	return func(in [2]float64) float64 {
		return math.Max(in[0], in[1])
	}
}

type Minimum types.Value[[2]float64, float64]
func NewMinimum() Minimum {
	return func(in [2]float64) float64 {
		return math.Min(in[0], in[1])
	}
}

type Bound types.Value[float64, float64]
/*
NewBound creates a closure that clamps an incoming value within fixed min/max thresholds.
*/
func NewBound(min, max float64) Bound {
	return func(in float64) float64 {
		return math.Max(min, math.Min(max, in))
	}
}

type SecondDifference types.Value[float64, float64]
/*
NewSecondDifference creates a stateful closure calculating the acceleration
of a time series (difference of differences).
*/
func NewSecondDifference() SecondDifference {
	var v1, v2 float64
	var count int
	return func(in float64) float64 {
		count++
		d := in - v1
		d2 := d - v2
		v2 = d
		v1 = in
		if count > 2 {
			return d2
		}
		return 0
	}
}

type Polarize types.Value[[2]float64, float64]
/*
NewPolarize creates a closure that splits a signed value into nonnegative components
and normalizes them against a configured scale.
Input is [value, scale]. Output is the normalized value.
*/
func NewPolarize() Polarize {
	return func(in [2]float64) float64 {
		val, scale := in[0], in[1]
		alpha := val
		if alpha < 0 {
			alpha = 0
		}
		beta := -val
		if beta < 0 {
			beta = 0
		}

		if scale > 0 {
			alpha = alpha / (alpha + scale)
			beta = beta / (beta + scale)
		}
		return alpha - beta
	}
}

type RelativeChange types.Value[float64, float64]
/*
NewRelativeChange creates a stateful closure calculating the relative difference
between the current and previous observations.
*/
func NewRelativeChange() RelativeChange {
	var previous float64
	var initialized bool
	return func(in float64) float64 {
		if !initialized {
			previous = in
			initialized = true
			return 0
		}
		if previous == 0 {
			previous = in
			return 0
		}
		change := (in - previous) / previous
		previous = in
		return change
	}
}
