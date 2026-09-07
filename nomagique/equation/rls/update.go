package rls

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Update owns delivery of the symmetric square-root rank-one posterior update. */
type Update struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

/* NewUpdate returns fresh posterior arrays and preserves the prior predictive facts. */
func NewUpdate() core.Primitive {
	return transport.NewMap(&Update{seed: transport.NewIO(core.From(map[string]core.Primitive{}))})
}

func (update *Update) Next(input core.Primitive) core.Primitive {
	result := core.Yield(update.seed, input,
		func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
			output, err := update.Posterior(fields)
			update.Error(err)
			return output
		}, update)

	if result != nil {
		update.current = result
	}
	return result
}

/* Posterior computes (root - gain*factor^T*alpha/gamma)/sqrt(lambda) directly. */
func (update *Update) Posterior(fields map[string]core.Primitive) (map[string]core.Primitive, error) {
	decoder := core.NewDecoder(fields)
	beta := core.Decode[[]float64](decoder, "beta")
	root := core.Decode[[][]float64](decoder, "root")
	factor := core.Decode[[]float64](decoder, "factor")
	lambda := core.Decode[float64](decoder, "lambda")
	innovation := core.Decode[float64](decoder, "target") - core.Decode[float64](decoder, "prediction")
	shape := core.Decode[float64](decoder, "noise_shape")
	noise := core.Decode[float64](decoder, "noise_scale")

	if err := decoder.Error(); err != nil {
		return nil, err
	}

	if len(root) != len(beta) || len(factor) != len(beta) {
		return nil, fmt.Errorf("%w: RLS update dimensions differ", core.ErrShape)
	}
	energy := 0.0

	for _, value := range factor {
		energy += value * value
	}

	alpha := lambda + energy

	if !(alpha > 0) || math.IsInf(alpha, 0) || math.IsNaN(innovation) || math.IsInf(innovation, 0) {
		return nil, fmt.Errorf("%w: invalid RLS innovation or information", core.ErrDomain)
	}
	rootLambda := math.Sqrt(lambda)
	denominator := alpha + rootLambda*math.Sqrt(alpha)
	gain := make([]float64, len(beta))
	coefficients := make([]float64, len(beta))
	posterior := make([][]float64, len(root))
	storage := make([]float64, len(root)*len(root))

	for row := range root {
		if len(root[row]) != len(beta) {
			return nil, fmt.Errorf("%w: RLS root must be square", core.ErrShape)
		}

		for column, coefficient := range root[row] {
			gain[row] += coefficient * factor[column]
		}
		gain[row] /= alpha
		coefficients[row] = beta[row] + gain[row]*innovation
		posterior[row] = storage[row*len(root) : (row+1)*len(root)]

		if math.IsNaN(coefficients[row]) || math.IsInf(coefficients[row], 0) {
			return nil, fmt.Errorf("%w: invalid RLS coefficient", core.ErrDomain)
		}

		for column, coefficient := range root[row] {
			posterior[row][column] = (coefficient - gain[row]*(alpha/denominator)*factor[column]) / rootLambda

			if math.IsNaN(posterior[row][column]) || math.IsInf(posterior[row][column], 0) {
				return nil, fmt.Errorf("%w: invalid RLS root", core.ErrDomain)
			}
		}
	}
	noise = lambda*noise + 0.5*innovation*innovation/alpha

	if math.IsNaN(noise) || math.IsInf(noise, 0) {
		return nil, fmt.Errorf("%w: invalid RLS noise scale", core.ErrDomain)
	}
	output := make(map[string]core.Primitive, len(fields)+9)

	for name, value := range fields {
		output[name] = value
	}
	output["alpha"], output["innovation"] = core.From(alpha), core.From(innovation)
	output["root_lambda"], output["gamma_denominator"], output["gain"] = core.From(rootLambda), core.From(denominator), core.From(gain)
	output["beta"], output["root"] = core.From(coefficients), core.From(posterior)
	output["noise_shape"], output["noise_scale"] = core.From(lambda*shape+0.5), core.From(noise)
	return output, nil
}

func (update *Update) Read() any { return core.To[any](update.current) }
