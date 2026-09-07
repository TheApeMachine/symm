package equation

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* SamplingVariance applies specificity debt, with one observation as the sampling floor. */
func SamplingVariance(depth, contextLength, support, variance float64) (float64, error) {
	if !(depth <= contextLength) {
		return 0, fmt.Errorf("%w: matched depth exceeds context length", core.ErrDomain)
	}
	return variance / math.Max(1, support/(1+(contextLength-depth))), nil
}

/* samplingVariance binds the named Primitive protocol to the numeric equation. */
type samplingVariance struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

/* NewSamplingVariance retains the record-based connection for Primitive composition. */
func NewSamplingVariance() core.Primitive {
	return transport.NewMap(&samplingVariance{seed: transport.NewIO(core.From(0.0))})
}

func (variance *samplingVariance) Next(input core.Primitive) core.Primitive {
	result := core.Yield(variance.seed, input,
		func(_ float64, fields map[string]core.Primitive) float64 {
			decoder := core.NewDecoder(fields)
			depth, length := core.Decode[float64](decoder, "depth"), core.Decode[float64](decoder, "context_length")
			support, value := core.Decode[float64](decoder, "support"), core.Decode[float64](decoder, "variance")

			if err := decoder.Error(); err != nil {
				variance.Error(err)
				return 0
			}
			result, err := SamplingVariance(depth, length, support, value)
			variance.Error(err)
			return result
		}, variance)

	if result != nil {
		variance.current = result
	}
	return result
}

func (variance *samplingVariance) Read() any { return core.To[any](variance.current) }
