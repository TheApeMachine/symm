package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var Tanh types.Value[float64, float64] = math.Tanh
var Absolute types.Value[float64, float64] = math.Abs
var Atanh types.Value[float64, float64] = math.Atanh
var Erfc types.Value[float64, float64] = math.Erfc
var Exp types.Value[float64, float64] = math.Exp
var Floor types.Value[float64, float64] = math.Floor
var Log types.Value[float64, float64] = math.Log
var Sqrt types.Value[float64, float64] = math.Sqrt

var Reciprocal types.Value[float64, float64] = func(in float64) float64 {
	if in == 0 {
		return 0 
	}
	return 1.0 / in
}

var Square types.Value[float64, float64] = func(in float64) float64 {
	return in * in
}

var Sign types.Value[float64, float64] = func(in float64) float64 {
	if in < 0 {
		return -1
	} else if in > 0 {
		return 1
	}
	return 0
}

var Negate types.Value[float64, float64] = func(in float64) float64 {
	return -in
}

var Maximum types.Value[[2]float64, float64] = func(in [2]float64) float64 {
	return math.Max(in[0], in[1])
}

var Minimum types.Value[[2]float64, float64] = func(in [2]float64) float64 {
	return math.Min(in[0], in[1])
}

/*
NewBound creates a closure that clamps an incoming value within fixed min/max thresholds.
*/
func NewBound(min, max float64) types.Value[float64, float64] {
	return func(in float64) float64 {
		return math.Max(min, math.Min(max, in))
	}
}

/*
NewSecondDifference creates a stateful closure calculating the acceleration
of a time series (difference of differences).
*/
func NewSecondDifference() types.Value[float64, float64] {
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

/*
NewPolarize creates a closure that splits a signed value into nonnegative components 
and normalizes them against a configured scale.
Input is [value, scale]. Output is the normalized value.
*/
func NewPolarize() types.Value[[2]float64, float64] {
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

/*
NewRelativeChange creates a stateful closure calculating the relative difference
between the current and previous observations.
*/
func NewRelativeChange() types.Value[float64, float64] {
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

