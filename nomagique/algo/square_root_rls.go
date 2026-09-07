package algo

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation/rls"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* SquareRootRLS owns the four posterior fields and its configured sources. */
type SquareRootRLS struct {
	core.PrimitiveError
	initial, forgetting, prediction, update core.Primitive
	state                                   [4]core.Primitive
	seed                                    *transport.IO
	current                                 core.Primitive
}

var posteriorFields = [4]string{"beta", "root", "noise_shape", "noise_scale"}

/*
NewSquareRootRLS predicts each {design,target} before training. A {design} query
never trains on an earlier target. Only a successful update commits the four
posterior fields; invalid input does not reset or partially train the model.
*/
func NewSquareRootRLS(initial, forgetting core.Primitive) core.Primitive {
	return transport.NewMap(&SquareRootRLS{
		initial: initial, forgetting: forgetting,
		prediction: rls.NewPrediction(store.NewConstant(core.From(1.0))), update: rls.NewUpdate(),
		seed: transport.NewIO(core.From(map[string]core.Primitive{})),
	})
}

func (learner *SquareRootRLS) Next(input core.Primitive) core.Primitive {
	result := core.Yield(learner.seed, input,
		func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
			output, err := learner.Step(fields)
			learner.Error(err)
			return output
		}, learner)

	if result != nil {
		learner.current = result
	}
	return result
}

/* Step prepares one prior forecast and commits a validated posterior when labeled. */
func (learner *SquareRootRLS) Step(fields map[string]core.Primitive) (map[string]core.Primitive, error) {
	design, err := core.Field[[]float64](fields, "design")

	if err != nil {
		return nil, err
	}
	lambda, err := transport.Evaluate[float64](learner.forgetting, core.From(fields))

	if err != nil {
		return nil, err
	}

	if !(lambda > 0 && lambda <= 1) {
		return nil, fmt.Errorf("%w: RLS forgetting factor must be in (0,1]", core.ErrDomain)
	}

	for _, feature := range design {
		if math.IsNaN(feature) || math.IsInf(feature, 0) {
			return nil, fmt.Errorf("%w: invalid RLS design", core.ErrDomain)
		}
	}

	if learner.state[0] == nil {
		initial, err := transport.Evaluate[map[string]core.Primitive](learner.initial, core.From(fields))

		if err != nil {
			return nil, err
		}

		for index, name := range posteriorFields {
			learner.state[index] = initial[name]
		}
	}
	input := make(map[string]core.Primitive, len(fields)+6)

	for name, value := range fields {
		input[name] = value
	}

	for index, name := range posteriorFields {
		input[name] = learner.state[index]
	}
	_, observe := fields["target"]
	input["lambda"], input["observe"] = core.From(lambda), core.From(observe)
	forecast, err := transport.Evaluate[map[string]core.Primitive](learner.prediction, core.From(input))

	if err != nil || !observe {
		return forecast, err
	}
	posterior, err := transport.Evaluate[map[string]core.Primitive](learner.update, core.From(forecast))

	if err != nil {
		return nil, err
	}

	for index, name := range posteriorFields {
		learner.state[index] = posterior[name]
	}
	return posterior, nil
}

func (learner *SquareRootRLS) Read() any { return core.To[any](learner.current) }
