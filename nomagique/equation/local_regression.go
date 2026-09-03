package equation

import (
	"math"
)

/*
LocalRegression is a streaming OLS slope estimator that operates on
divergence time-series. It tracks sufficient statistics for a windowed
linear regression of value vs. elapsed time (in seconds), computing the
slope and a signal-to-noise ratio from the residual variance.

This is a pure v2 equation: zero Frame, zero Symbol, zero MustIntern.
*/
type LocalRegression struct {
	// Online sufficient statistics for OLS: slope = S_xy / S_xx
	sumX   float64 // sum of elapsed-seconds
	sumY   float64 // sum of values
	sumXX  float64 // sum of (elapsed)^2
	sumXY  float64 // sum of (elapsed * value)
	sumYY  float64 // sum of value^2
	count  float64
	origin int64 // first timestamp in nanoseconds
	hasOrigin bool
}

func (regression *LocalRegression) Step(value float64, timestampNano int64, horizon int64) {
	if !regression.hasOrigin {
		regression.origin = timestampNano
		regression.hasOrigin = true
	}

	elapsed := float64(timestampNano-regression.origin) / 1e9

	regression.sumX += elapsed
	regression.sumY += value
	regression.sumXX += elapsed * elapsed
	regression.sumXY += elapsed * value
	regression.sumYY += value * value
	regression.count++
}

func (regression *LocalRegression) Slope() (float64, bool) {
	if regression.count < 3 {
		return 0, false
	}

	meanX := regression.sumX / regression.count
	meanY := regression.sumY / regression.count
	sxx := regression.sumXX - regression.count*meanX*meanX
	sxy := regression.sumXY - regression.count*meanX*meanY

	if sxx <= 0 {
		return 0, false
	}

	slope := sxy / sxx

	if math.IsNaN(slope) || math.IsInf(slope, 0) {
		return 0, false
	}

	return slope, true
}

func (regression *LocalRegression) SNR() (float64, bool) {
	if regression.count < 4 {
		return 0, false
	}

	meanX := regression.sumX / regression.count
	meanY := regression.sumY / regression.count
	sxx := regression.sumXX - regression.count*meanX*meanX
	sxy := regression.sumXY - regression.count*meanX*meanY
	syy := regression.sumYY - regression.count*meanY*meanY

	if sxx <= 0 || syy <= 0 {
		return 0, false
	}

	slope := sxy / sxx
	ssResidual := syy - slope*sxy

	if ssResidual <= 0 {
		return 0, false
	}

	residualVariance := ssResidual / (regression.count - 2)

	if residualVariance <= 0 {
		return 0, false
	}

	slopeVariance := residualVariance / sxx
	snr := (slope * slope) / slopeVariance

	if math.IsNaN(snr) || math.IsInf(snr, 0) {
		return 0, false
	}

	return snr, true
}
