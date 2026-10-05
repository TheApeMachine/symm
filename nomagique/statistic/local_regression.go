package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
LocalRegressionReading remains as the public result vocabulary for callers
that have not yet migrated their presentation layer. LocalRegression.Next no
longer puts this struct on the wire.
*/
type LocalRegressionReading struct {
	Slope        float64
	SlopeDefined bool
	SNR          float64
	SNRDefined   bool
	Count        float64
}

/*
LocalRegression is a streaming OLS slope estimator over a numeric event-time
coordinate.
*/
type LocalRegression struct {
	*core.PrimitiveError
	sumX      float64
	sumY      float64
	sumXX     float64
	sumXY     float64
	sumYY     float64
	count     float64
	origin    float64
	hasOrigin bool
	input     data.Map[string]
	output    data.Map[float64]
}

func NewLocalRegression() core.Primitive {
	output := data.NewOutputMap()
	output.Values["slope"] = 0
	output.Values["slope_snr"] = 0

	return &LocalRegression{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value", "at", "at"),
		output:         output,
	}
}

func (op *LocalRegression) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)
			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			value, valueOK := values.Values["value"]
			at, atOK := values.Values["at"]

			if !valueOK || !atOK {
				if !yield(arriving) {
					return
				}
				continue
			}

			if !op.hasOrigin {
				op.origin = at
				op.hasOrigin = true
			}

			elapsed := at - op.origin
			op.sumX += elapsed
			op.sumY += value
			op.sumXX += elapsed * elapsed
			op.sumXY += elapsed * value
			op.sumYY += value * value
			op.count++

			if op.count >= 3 {
				meanX := op.sumX / op.count
				meanY := op.sumY / op.count
				sxx := op.sumXX - op.count*meanX*meanX
				sxy := op.sumXY - op.count*meanX*meanY

				if sxx > 0 {
					slope := sxy / sxx
					op.output.Values["slope"] = slope

					if op.count >= 4 {
						syy := op.sumYY - op.count*meanY*meanY

						if syy > 0 {
							ssResidual := syy - slope*sxy

							if ssResidual > 0 {
								residualVariance := ssResidual / (op.count - 2)

								if residualVariance > 0 {
									op.output.Values["slope_snr"] = (slope * slope) / (residualVariance / sxx)
								}
							}
						}
					}

					for range adapter.Next(data.NewValue(op.output)) {
					}
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
