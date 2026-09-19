package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

type Tanh types.Value[float64, float64]
func NewTanh(operands ...types.Float) Tanh {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return math.Tanh(val)
	}
}

type Absolute types.Value[float64, float64]
func NewAbsolute(operands ...types.Float) Absolute {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return math.Abs(val)
	}
}

type Atanh types.Value[float64, float64]
func NewAtanh(operands ...types.Float) Atanh {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return math.Atanh(val)
	}
}

type Erfc types.Value[float64, float64]
func NewErfc(operands ...types.Float) Erfc {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return math.Erfc(val)
	}
}

type Exp types.Value[float64, float64]
func NewExp(operands ...types.Float) Exp {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return math.Exp(val)
	}
}

type Floor types.Value[float64, float64]
func NewFloor(operands ...types.Float) Floor {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return math.Floor(val)
	}
}

type Log types.Value[float64, float64]
func NewLog(operands ...types.Float) Log {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return math.Log(val)
	}
}

type Sqrt types.Value[float64, float64]
func NewSqrt(operands ...types.Float) Sqrt {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return math.Sqrt(val)
	}
}

type Reciprocal types.Value[float64, float64]
func NewReciprocal(operands ...types.Float) Reciprocal {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		if val == 0 {
			return 0
		}
		return core.Unit / val
	}
}

type Square types.Value[float64, float64]
func NewSquare(operands ...types.Float) Square {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return val * val
	}
}

type Sign types.Value[float64, float64]
func NewSign(operands ...types.Float) Sign {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		if val < 0 {
			return -1
		}
		if val > 0 {
			return 1
		}
		return 0
	}
}

type Negate types.Value[float64, float64]
func NewNegate(operands ...types.Float) Negate {
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		return -val
	}
}

type Maximum types.Value[[2]float64, float64]
func NewMaximum(operands ...types.Float) Maximum {
	return func(in [2]float64) float64 {
		if len(operands) >= 2 {
			l := in[0]
			r := in[1]
			if operands[0] != nil {
				l = operands[0](in)
			}
			if operands[1] != nil {
				r = operands[1](in)
			}
			return math.Max(l, r)
		}
		return math.Max(in[0], in[1])
	}
}

type Minimum types.Value[[2]float64, float64]
func NewMinimum(operands ...types.Float) Minimum {
	return func(in [2]float64) float64 {
		if len(operands) >= 2 {
			l := in[0]
			r := in[1]
			if operands[0] != nil {
				l = operands[0](in)
			}
			if operands[1] != nil {
				r = operands[1](in)
			}
			return math.Min(l, r)
		}
		return math.Min(in[0], in[1])
	}
}

type Bound types.Value[float64, float64]
/*
NewBound creates a closure that clamps an incoming value within fixed min/max thresholds.
*/
func NewBound(min, max types.Float) Bound {
	return func(in float64) float64 {
		mi := -math.MaxFloat64
		if min != nil {
			mi = min(in)
		}
		ma := math.MaxFloat64
		if max != nil {
			ma = max(in)
		}
		return math.Max(mi, math.Min(ma, in))
	}
}

type SecondDifference types.Value[float64, float64]
/*
NewSecondDifference creates a stateful closure calculating the acceleration
of a time series (difference of differences).
*/
func NewSecondDifference(operands ...types.Float) SecondDifference {
	var v1, v2 float64
	var count int
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		count++
		d := val - v1
		d2 := d - v2
		v2 = d
		v1 = val
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
func NewPolarize(operands ...types.Float) Polarize {
	return func(in [2]float64) float64 {
		val, scale := in[0], in[1]
		if len(operands) >= 2 {
			if operands[0] != nil {
				val = operands[0](in)
			}
			if operands[1] != nil {
				scale = operands[1](in)
			}
		}
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
func NewRelativeChange(operands ...types.Float) RelativeChange {
	var previous float64
	var initialized bool
	return func(in float64) float64 {
		val := in
		if len(operands) > 0 && operands[0] != nil {
			val = operands[0](in)
		}
		if !initialized {
			previous = val
			initialized = true
			return 0
		}
		if previous == 0 {
			previous = val
			return 0
		}
		change := (val - previous) / previous
		previous = val
		return change
	}
}
