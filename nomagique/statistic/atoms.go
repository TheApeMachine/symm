package statistic

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewMean creates a stateful closure that tracks the running average.
*/
func NewMean() types.Value[float64, float64] {
	var count, sum float64
	return func(in float64) float64 {
		count++
		sum += in
		return sum / count
	}
}

/*
NewVariance creates a stateful closure calculating running sample variance using Welford's algorithm.
*/
func NewVariance() types.Value[float64, float64] {
	var count, mean, m2 float64
	return func(in float64) float64 {
		count++
		delta := in - mean
		mean += delta / count
		delta2 := in - mean
		m2 += delta * delta2

		if count > 1 {
			return m2 / (count - 1)
		}
		return 0
	}
}

/*
NewEMA creates a stateful Exponential Moving Average closure.
The smoothing factor (alpha) is permanently closed over.
*/
func NewEMA(alpha float64) types.Value[float64, float64] {
	var ema float64
	var initialized bool
	
	return func(in float64) float64 {
		if !initialized {
			ema = in
			initialized = true
			return ema
		}
		ema = (in * alpha) + (ema * (1 - alpha))
		return ema
	}
}

/*
NewCausalMean creates a stateful running average that yields the PRIOR mean 
before incorporating the current value.
*/
func NewCausalMean() types.Value[float64, float64] {
	var count, sum, prevMean float64
	return func(in float64) float64 {
		ret := prevMean
		count++
		sum += in
		prevMean = sum / count
		
		if count == 1 {
			return in // First observation baseline is itself
		}
		return ret
	}
}

/*
NewCausalVariance creates a stateful running variance that yields the PRIOR variance.
*/
func NewCausalVariance() types.Value[float64, float64] {
	var count, mean, m2, prevVar float64
	return func(in float64) float64 {
		ret := prevVar
		count++
		delta := in - mean
		mean += delta / count
		delta2 := in - mean
		m2 += delta * delta2

		if count > 1 {
			prevVar = m2 / (count - 1)
		}
		
		if count <= 2 {
			return 0
		}
		return ret
	}
}

/*
NewResidualBaseline satisfies the JSON graph by returning a causal mean.
*/
func NewResidualBaseline() types.Value[float64, float64] {
	return NewCausalMean()
}

/*
NewResidualDivergence creates a stateful closure returning the difference 
between the current value and the causal baseline.
*/
func NewResidualDivergence() types.Value[float64, float64] {
	causalMean := NewCausalMean()
	return func(in float64) float64 {
		return in - causalMean(in)
	}
}

/*
NewZScore creates a stateful closure that calculates the Z-Score of an incoming stream.
It encapsulates its own causal mean and variance, completely eliminating the need for DTOs.
*/
func NewZScore() types.Value[float64, float64] {
	causalMean := NewCausalMean()
	causalVar := NewCausalVariance()

	return func(in float64) float64 {
		baseline := causalMean(in)
		variance := causalVar(in)
		
		if variance <= 0 {
			return 0
		}
		
		return (in - baseline) / math.Sqrt(variance)
	}
}

