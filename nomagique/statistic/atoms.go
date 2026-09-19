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
func NewMean() Mean {
	var count, sum float64
	return func(in float64) float64 {
		count++
		sum += in
		return sum / count
	}
}

type Variance types.Value[float64, float64]
/*
NewVariance creates a stateful closure calculating running sample variance using Welford's algorithm.
*/
func NewVariance() Variance {
	var count, mean, m2 float64
	return func(in float64) float64 {
		count++
		delta := in - mean
		mean += delta / count
		delta2 := in - mean
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
func NewEMA(alpha float64) EMA {
	var ema float64
	var initialized bool

	return func(in float64) float64 {
		if !initialized {
			ema = in
			initialized = true
			return ema
		}
		ema = (in * alpha) + (ema * (core.Unit - alpha))
		return ema
	}
}

type CausalMean types.Value[float64, float64]
/*
NewCausalMean creates a stateful running average that yields the PRIOR mean
before incorporating the current value.
*/
func NewCausalMean() CausalMean {
	var count, sum, prevMean float64
	return func(in float64) float64 {
		ret := prevMean
		count++
		sum += in
		prevMean = sum / count

		if count == core.Unit {
			return in // First observation baseline is itself
		}

		return ret
	}
}

type CausalVariance types.Value[float64, float64]
/*
NewCausalVariance creates a stateful running variance that yields the PRIOR variance.
*/
func NewCausalVariance() CausalVariance {
	var count, mean, m2, prevVar float64
	return func(in float64) float64 {
		ret := prevVar
		count++
		delta := in - mean
		mean += delta / count
		delta2 := in - mean
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
func NewResidualBaseline() ResidualBaseline {
	causalMean := NewCausalMean()
	return func(in float64) float64 { return causalMean(in) }
}

type ResidualDivergence types.Value[float64, float64]
/*
NewResidualDivergence creates a stateful closure returning the difference
between the current value and the causal baseline.
*/
func NewResidualDivergence() ResidualDivergence {
	causalMean := NewCausalMean()
	return func(in float64) float64 {
		return in - causalMean(in)
	}
}

type ZScore types.Value[float64, float64]
/*
NewZScore creates a stateful closure that calculates the Z-Score of an incoming stream.
It encapsulates its own causal mean and variance, completely eliminating the need for DTOs.
*/
func NewZScore() ZScore {
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

type Threshold types.Value[float64, float64]
/*
NewThreshold creates a state-free closure that maps a rank (0.0 to 1.0) to a target value.
It uses a threshold band to decide whether to output the min, max, or rest value.
*/
func NewThreshold(band, rest, lower, upper float64) Threshold {
	return func(rank float64) float64 {
		if rank < band {
			return upper
		} else if rank > 1.0-band {
			return lower
		}
		return rest
	}
}
