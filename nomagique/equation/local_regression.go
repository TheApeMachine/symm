package equation

import (
	"iter"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
LocalRegression is a streaming OLS slope estimator. It tracks sufficient
statistics for a linear regression of value vs elapsed time in seconds.
*/
type LocalRegression struct {
	core.Base[Price, LocalRegressionReading]
	sumX      float64
	sumY      float64
	sumXX     float64
	sumXY     float64
	sumYY     float64
	count     float64
	origin    int64
	hasOrigin bool
}

/*
LocalRegressionReading is the slope and SNR after one observation.
*/
type LocalRegressionReading struct {
	Slope        float64
	SlopeDefined bool
	SNR          float64
	SNRDefined   bool
	Count        float64
}

func NewLocalRegression() *LocalRegression {
	return &LocalRegression{}
}

func (op *LocalRegression) Next(
	in iter.Seq[core.Primitive[Price, Price]],
) iter.Seq[core.Primitive[LocalRegressionReading, LocalRegressionReading]] {
	return func(yield func(core.Primitive[LocalRegressionReading, LocalRegressionReading]) bool) {
		for arriving := range in {
			point := arriving.Read()
			op.step(point.Value, point.At)
			slope, slopeDefined := op.Slope()
			snr, snrDefined := op.SNR()

			if !yield(op.Carrier(LocalRegressionReading{
				Slope:        slope,
				SlopeDefined: slopeDefined,
				SNR:          snr,
				SNRDefined:   snrDefined,
				Count:        op.count,
			})) {
				return
			}
		}
	}
}

func (op *LocalRegression) step(value float64, timestampNano int64) {
	if !op.hasOrigin {
		op.origin = timestampNano
		op.hasOrigin = true
	}

	elapsed := float64(timestampNano-op.origin) / float64(time.Second)
	op.sumX += elapsed
	op.sumY += value
	op.sumXX += elapsed * elapsed
	op.sumXY += elapsed * value
	op.sumYY += value * value
	op.count++
}

func (op *LocalRegression) Slope() (float64, bool) {
	if op.count < 3 {
		return 0, false
	}

	meanX := op.sumX / op.count
	meanY := op.sumY / op.count
	sxx := op.sumXX - op.count*meanX*meanX
	sxy := op.sumXY - op.count*meanX*meanY

	if sxx <= 0 {
		return 0, false
	}

	return sxy / sxx, true
}

func (op *LocalRegression) SNR() (float64, bool) {
	if op.count < 4 {
		return 0, false
	}

	meanX := op.sumX / op.count
	meanY := op.sumY / op.count
	sxx := op.sumXX - op.count*meanX*meanX
	sxy := op.sumXY - op.count*meanX*meanY
	syy := op.sumYY - op.count*meanY*meanY

	if sxx <= 0 || syy <= 0 {
		return 0, false
	}

	slope := sxy / sxx
	ssResidual := syy - slope*sxy

	if ssResidual <= 0 {
		return 0, false
	}

	residualVariance := ssResidual / (op.count - 2)

	if residualVariance <= 0 {
		return 0, false
	}

	return (slope * slope) / (residualVariance / sxx), true
}
