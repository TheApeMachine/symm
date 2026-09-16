package statistic

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
)

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
	*core.PrimitiveError

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

func NewLocalRegression() *LocalRegression {
	return &LocalRegression{PrimitiveError: core.NewPrimitiveError()}
}

func (localRegression *LocalRegression) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			price := (*temporal.Price)(arriving)

			if !localRegression.hasOrigin {
				localRegression.origin = price.At
				localRegression.hasOrigin = true
			}

			elapsed := float64(price.At-localRegression.origin) / float64(time.Second)
			localRegression.sumX += elapsed
			localRegression.sumY += price.Value
			localRegression.sumXX += elapsed * elapsed
			localRegression.sumXY += elapsed * price.Value
			localRegression.sumYY += price.Value * price.Value
			localRegression.count++

			localRegression.out = LocalRegressionReading{Count: localRegression.count}

			if localRegression.count >= 3 {
				meanX := localRegression.sumX / localRegression.count
				meanY := localRegression.sumY / localRegression.count
				sxx := localRegression.sumXX - localRegression.count*meanX*meanX
				sxy := localRegression.sumXY - localRegression.count*meanX*meanY

				if sxx > 0 {
					localRegression.out.Slope = sxy / sxx
					localRegression.out.SlopeDefined = true

					if localRegression.count >= 4 {
						syy := localRegression.sumYY - localRegression.count*meanY*meanY

						if syy > 0 {
							ssResidual := syy - localRegression.out.Slope*sxy

							if ssResidual > 0 {
								residualVariance := ssResidual / (localRegression.count - 2)

								if residualVariance > 0 {
									localRegression.out.SNR = (localRegression.out.Slope * localRegression.out.Slope) / (residualVariance / sxx)
									localRegression.out.SNRDefined = true
								}
							}
						}
					}
				}
			}

			if !yield(unsafe.Pointer(&localRegression.out)) {
				return
			}
		}
	}
}
