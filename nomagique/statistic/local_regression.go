package statistic

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type Point struct {
	At    int64
	Value float64
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

/*
LocalRegression is a streaming OLS slope estimator. It tracks sufficient
statistics for a linear regression of value vs elapsed time in seconds.
*/
type LocalRegression struct {
	err       error
	sumX      float64
	sumY      float64
	sumXX     float64
	sumXY     float64
	sumYY     float64
	count     float64
	origin    int64
	hasOrigin bool
	out       LocalRegressionReading
}

func NewLocalRegression() core.Primitive {
	return &LocalRegression{}
}

func (op *LocalRegression) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			point := (*Point)(arriving)

			if !op.hasOrigin {
				op.origin = point.At
				op.hasOrigin = true
			}

			elapsed := float64(point.At-op.origin) / float64(time.Second)
			op.sumX += elapsed
			op.sumY += point.Value
			op.sumXX += elapsed * elapsed
			op.sumXY += elapsed * point.Value
			op.sumYY += point.Value * point.Value
			op.count++

			op.out = LocalRegressionReading{Count: op.count}

			if op.count >= 3 {
				meanX := op.sumX / op.count
				meanY := op.sumY / op.count
				sxx := op.sumXX - op.count*meanX*meanX
				sxy := op.sumXY - op.count*meanX*meanY

				if sxx > 0 {
					op.out.Slope = sxy / sxx
					op.out.SlopeDefined = true

					if op.count >= 4 {
						syy := op.sumYY - op.count*meanY*meanY

						if syy > 0 {
							ssResidual := syy - op.out.Slope*sxy

							if ssResidual > 0 {
								residualVariance := ssResidual / (op.count - 2)

								if residualVariance > 0 {
									op.out.SNR = (op.out.Slope * op.out.Slope) / (residualVariance / sxx)
									op.out.SNRDefined = true
								}
							}
						}
					}
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *LocalRegression) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
