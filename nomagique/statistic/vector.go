package statistic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewVectorEMA creates a stateful Exponential Moving Average closure for vectors.
The smoothing factor (alpha) is permanently closed over.
*/
type VectorEMA types.Value[[]float64, []float64]
func NewVectorEMA(alpha float64) VectorEMA {
	var ema []float64
	var initialized bool

	return func(in []float64) []float64 {
		if !initialized {
			ema = make([]float64, len(in))
			copy(ema, in)
			initialized = true
			return ema
		}
		
		for i := range in {
			ema[i] = (in[i] * alpha) + (ema[i] * (core.Unit - alpha))
		}
		return ema
	}
}
