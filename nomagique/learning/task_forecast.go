package learning

import (
	"fmt"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* RLSOutput is a wire projection, not a parallel learner or execution interface. */
type RLSOutput struct {
	Value            float64
	Scale            float64
	DegreesOfFreedom float64
	Ready            bool
	Innovation       float64
	Reset            bool
}

/* taskForecast deliberately omits target: querying cannot train the head. */
func taskForecast(graph core.Primitive, features []float64) (RLSOutput, error) {
	fields, err := transport.Evaluate[map[string]core.Primitive](graph, core.Record(map[string]any{"features": features}))
	if err != nil {
		return RLSOutput{}, err
	}
	decoder := core.NewDecoder(fields)
	output := RLSOutput{
		Value:            core.Decode[float64](decoder, "prediction"),
		Scale:            core.Decode[float64](decoder, "scale"),
		DegreesOfFreedom: core.Decode[float64](decoder, "degrees_of_freedom"),
		Ready:            core.Decode[bool](decoder, "ready"),
	}
	return output, decoder.Error()
}

/* taskCoefficients projects the graph's posterior into the dense manifold head. */
func taskCoefficients(fields map[string]core.Primitive, destination []float64) (float64, error) {
	beta, err := core.Field[[]float64](fields, "beta")
	if err != nil {
		return 0, err
	}
	if len(beta) != len(destination)+1 {
		return 0, fmt.Errorf("resonance: coefficient width %d does not match head %d", len(beta), len(destination))
	}
	copy(destination, beta[1:])
	return beta[0], nil
}
