package statistic

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

type Mean types.Value[float64, float64]
/*
NewMean creates a stateful closure that tracks the running average.
*/
func NewMean(values ...types.Float) Mean {
	var count, sum float64
	return func(in float64) float64 {
		val := in
		if len(values) > 0 && values[0] != nil {
			val = values[0](in)
		}
		count++
		sum += val
		return sum / count
	}
}

type Variance types.Value[float64, float64]
/*
NewVariance creates a stateful closure calculating running sample variance using Welford's algorithm.
*/
func NewVariance(values ...types.Float) Variance {
	var count, mean, m2 float64
	return func(in float64) float64 {
		val := in
		if len(values) > 0 && values[0] != nil {
			val = values[0](in)
		}
		count++
		delta := val - mean
		mean += delta / count
		delta2 := val - mean
		m2 += delta * delta2

		if count > core.Unit {
			return m2 / (count - core.Unit)
		}
		return 0
	}
}

type EMA types.Value[float64, float64]
/*
NewEMA creates a stateful Exponential Moving Average closure.
The smoothing factor (alpha) is permanently closed over.
*/
func NewEMA(alpha types.Float) EMA {
	var ema float64
	var initialized bool

	return func(in float64) float64 {
		a := 0.0
		if alpha != nil {
			a = alpha(in)
		}
		if a <= 0 {
			return in
		}
		if !initialized {
			ema = in
			initialized = true
			return ema
		}
		ema = (in * a) + (ema * (core.Unit - a))
		return ema
	}
}

type CausalMean types.Value[float64, float64]
/*
NewCausalMean creates a stateful running average that yields the PRIOR mean
before incorporating the current value.
*/
func NewCausalMean(values ...types.Float) CausalMean {
	var count, sum, prevMean float64
	return func(in float64) float64 {
		val := in
		if len(values) > 0 && values[0] != nil {
			val = values[0](in)
		}
		ret := prevMean
		count++
		sum += val
		prevMean = sum / count

		if count == core.Unit {
			return val
		}

		return ret
	}
}

type CausalVariance types.Value[float64, float64]
/*
NewCausalVariance creates a stateful running variance that yields the PRIOR variance.
*/
func NewCausalVariance(values ...types.Float) CausalVariance {
	var count, mean, m2, prevVar float64
	return func(in float64) float64 {
		val := in
		if len(values) > 0 && values[0] != nil {
			val = values[0](in)
		}
		ret := prevVar
		count++
		delta := val - mean
		mean += delta / count
		delta2 := val - mean
		m2 += delta * delta2

		if count > core.Unit {
			prevVar = m2 / (count - core.Unit)
		}

		if count <= 2*core.Unit {
			return 0
		}
		return ret
	}
}

type ResidualBaseline types.Value[float64, float64]
/*
NewResidualBaseline satisfies the JSON graph by returning a causal mean.
*/
func NewResidualBaseline(values ...types.Float) ResidualBaseline {
	causalMean := NewCausalMean(values...)
	return func(in float64) float64 { return causalMean(in) }
}

type ResidualDivergence types.Value[float64, float64]
/*
NewResidualDivergence creates a stateful closure returning the difference
between the current value and the causal baseline.
*/
func NewResidualDivergence(values ...types.Float) ResidualDivergence {
	causalMean := NewCausalMean(values...)
	return func(in float64) float64 {
		val := in
		if len(values) > 0 && values[0] != nil {
			val = values[0](in)
		}
		return val - causalMean(in)
	}
}

type ZScore types.Value[float64, float64]
/*
NewZScore creates a stateful closure that calculates the Z-Score of an incoming stream.
It encapsulates its own causal mean and variance, completely eliminating the need for DTOs.
*/
func NewZScore(values ...types.Float) ZScore {
	causalMean := NewCausalMean(values...)
	causalVar := NewCausalVariance(values...)

	return func(in float64) float64 {
		val := in
		if len(values) > 0 && values[0] != nil {
			val = values[0](in)
		}
		baseline := causalMean(in)
		variance := causalVar(in)

		if variance <= 0 {
			return 0
		}

		return (val - baseline) / math.Sqrt(variance)
	}
}

type Threshold types.Value[float64, float64]
/*
NewThreshold creates a state-free closure that maps a rank (0.0 to 1.0) to a target value.
It uses a threshold band to decide whether to output the min, max, or rest value.
*/
func NewThreshold(band, rest, lower, upper types.Float) Threshold {
	return func(rank float64) float64 {
		b := 0.0
		if band != nil {
			b = band(rank)
		}
		r := 0.0
		if rest != nil {
			r = rest(rank)
		}
		l := 0.0
		if lower != nil {
			l = lower(rank)
		}
		u := 0.0
		if upper != nil {
			u = upper(rank)
		}

		if rank < b {
			return u
		}

		if rank > 1.0-b {
			return l
		}

		return r
	}
}
