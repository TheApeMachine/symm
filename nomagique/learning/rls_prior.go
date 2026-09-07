package learning

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* rlsPrior owns the configured diagonal variance of an untrained affine model. */
type rlsPrior struct {
	core.PrimitiveError
	variance core.Primitive
	seed     *transport.IO
	current  core.Primitive
}

func newRLSPrior(variance core.Primitive) core.Primitive {
	return &rlsPrior{variance: variance, seed: transport.NewIO(core.From(map[string]core.Primitive{}))}
}

func (prior *rlsPrior) Next(input core.Primitive) core.Primitive {
	result := core.Yield(prior.seed, input,
		func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
			output, err := prior.Initialize(fields)
			prior.Error(err)
			return output
		}, prior)

	if result != nil {
		prior.current = result
	}
	return result
}

/* Initialize allocates one coefficient vector and its diagonal square root. */
func (prior *rlsPrior) Initialize(fields map[string]core.Primitive) (map[string]core.Primitive, error) {
	design, err := core.Field[[]float64](fields, "design")

	if err != nil {
		return nil, err
	}
	variance, err := transport.Evaluate[float64](prior.variance, core.From(fields))

	if err != nil {
		return nil, err
	}

	if !(variance > 0) || math.IsInf(variance, 0) {
		return nil, fmt.Errorf("%w: RLS prior variance must be finite and positive", core.ErrDomain)
	}
	size := len(design)
	root := make([][]float64, size)
	storage := make([]float64, size*size)

	for index := range root {
		root[index] = storage[index*size : (index+1)*size]
		root[index][index] = math.Sqrt(variance)
	}
	return map[string]core.Primitive{
		"beta": core.From(make([]float64, size)), "root": core.From(root),
		"noise_shape": core.From(0.0), "noise_scale": core.From(0.0),
	}, nil
}

func (prior *rlsPrior) Read() any { return core.To[any](prior.current) }
