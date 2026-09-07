package rls

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Prediction owns configured observation-noise multiplicity and typed projection. */
type Prediction struct {
	core.PrimitiveError
	observations core.Primitive
	seed         *transport.IO
	current      core.Primitive
}

/* NewPrediction forecasts from the supplied posterior before any model update. */
func NewPrediction(observationCount core.Primitive) core.Primitive {
	return transport.NewMap(&Prediction{
		observations: observationCount, seed: transport.NewIO(core.From(map[string]core.Primitive{})),
	})
}

func (prediction *Prediction) Next(input core.Primitive) core.Primitive {
	result := core.Yield(prediction.seed, input,
		func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
			output, err := prediction.Project(fields)
			prediction.Error(err)
			return output
		}, prediction)

	if result != nil {
		prediction.current = result
	}
	return result
}

/* Project multiplies the root's transpose implicitly, retaining only its factor. */
func (prediction *Prediction) Project(fields map[string]core.Primitive) (map[string]core.Primitive, error) {
	decoder := core.NewDecoder(fields)
	beta := core.Decode[[]float64](decoder, "beta")
	design := core.Decode[[]float64](decoder, "design")
	root := core.Decode[[][]float64](decoder, "root")
	shape := core.Decode[float64](decoder, "noise_shape")
	noise := core.Decode[float64](decoder, "noise_scale")

	if err := decoder.Error(); err != nil {
		return nil, err
	}

	if len(beta) != len(design) || len(root) != len(design) {
		return nil, fmt.Errorf("%w: RLS prediction coefficient, root and design dimensions differ", core.ErrShape)
	}
	factor := make([]float64, len(design))
	value := 0.0

	for row, feature := range design {
		if len(root[row]) != len(design) {
			return nil, fmt.Errorf("%w: RLS root must be square", core.ErrShape)
		}
		value += beta[row] * feature

		for column, coefficient := range root[row] {
			factor[column] += coefficient * feature
		}
	}
	output := make(map[string]core.Primitive, len(fields)+6)

	for name, field := range fields {
		output[name] = field
	}
	output["prediction"], output["factor"] = core.From(value), core.From(factor)
	output["scale"], output["degrees_of_freedom"], output["ready"] = core.From(0.0), core.From(0.0), core.From(false)

	if !(shape > 0 && noise > 0) {
		return output, nil
	}
	observations, err := transport.Evaluate[float64](prediction.observations, core.From(fields))

	if err != nil {
		return nil, err
	}
	energy := 0.0

	for _, value := range factor {
		energy += value * value
	}
	variance := (noise / shape) * (observations + energy)

	if !(variance > 0) || math.IsNaN(variance) || math.IsInf(variance, 0) {
		return nil, fmt.Errorf("%w: RLS predictive variance %g", core.ErrDomain, variance)
	}
	output["predictive_variance"] = core.From(variance)
	output["scale"], output["degrees_of_freedom"], output["ready"] = core.From(math.Sqrt(variance)), core.From(2*shape), core.From(true)
	return output, nil
}

func (prediction *Prediction) Read() any { return core.To[any](prediction.current) }
