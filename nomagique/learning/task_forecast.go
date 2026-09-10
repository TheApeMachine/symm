package learning

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/algo"
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
func taskForecast(learner *RLS, features []float64) (RLSOutput, error) {
	reading, err := transport.Evaluate(learner, transport.Values(Sample{Features: features}))

	if err != nil {
		return RLSOutput{}, err
	}

	return RLSOutput{
		Value:            reading.Prediction,
		Scale:            reading.Scale,
		DegreesOfFreedom: reading.DegreesOfFreedom,
		Ready:            reading.Ready,
		Innovation:       reading.Innovation,
	}, nil
}

/* taskCoefficients projects the posterior into the dense manifold head. */
func taskCoefficients(reading algo.Reading, destination []float64) (float64, error) {
	if len(reading.Beta) != len(destination)+1 {
		return 0, fmt.Errorf(
			"resonance: coefficient width %d does not match head %d",
			len(reading.Beta),
			len(destination),
		)
	}

	copy(destination, reading.Beta[1:])
	return reading.Beta[0], nil
}
