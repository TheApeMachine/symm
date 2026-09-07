package learning

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* RLS owns the configured feature dimension and affine design construction. */
type RLS struct {
	core.PrimitiveError
	dimension core.Primitive
	seed      *transport.IO
	current   core.Primitive
}

/*
NewRLS supplies an affine intercept and the zero-mean diagonal coefficient prior.
All configuration remains Primitive-valued and is evaluated at delivery time.
Prediction is prior to the optional target's update, as owned by SquareRootRLS.
*/
func NewRLS(dimension, variance, forgetting core.Primitive) core.Primitive {
	return transport.NewMap(transport.NewPipe(
		&RLS{dimension: dimension, seed: transport.NewIO(core.From(map[string]core.Primitive{}))},
		algo.NewSquareRootRLS(newRLSPrior(variance), forgetting),
	))
}

func (learner *RLS) Next(input core.Primitive) core.Primitive {
	result := core.Yield(learner.seed, input,
		func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
			output, err := learner.Prepare(fields)
			learner.Error(err)
			return output
		}, learner)

	if result != nil {
		learner.current = result
	}
	return result
}

/* Prepare decodes a feature vector once and prepends the affine intercept. */
func (learner *RLS) Prepare(fields map[string]core.Primitive) (map[string]core.Primitive, error) {
	features, err := core.Field[[]float64](fields, "features")

	if err != nil {
		return nil, err
	}
	dimension, err := transport.Evaluate[float64](learner.dimension, core.From(fields))

	if err != nil {
		return nil, err
	}

	if !(dimension > 0) || dimension != float64(len(features)) {
		return nil, fmt.Errorf("%w: RLS expected %g features, received %d", core.ErrShape, dimension, len(features))
	}
	design := make([]float64, len(features)+1)
	design[0] = 1
	copy(design[1:], features)
	output := make(map[string]core.Primitive, len(fields)+3)

	for name, value := range fields {
		output[name] = value
	}
	output["dimension"], output["feature_count"] = core.From(dimension), core.From(float64(len(features)))
	output["design"] = core.From(design)
	return output, nil
}

func (learner *RLS) Read() any { return core.To[any](learner.current) }
