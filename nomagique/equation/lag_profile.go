package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
LagEstimator consumes borrowed, already decoded return paths for one timestamp
offset. It must finish reading them before returning and publish owned result
fields. Estimators and diagnostic compositions implement the same operation.
*/
type LagEstimator interface {
	core.Primitive
	Estimate(left, right *LogReturns, lag int64) (map[string]core.Primitive, error)
}

/* LagProfile owns the configured estimator and exact discrete search coordinates. */
type LagProfile struct {
	core.PrimitiveError
	estimator     LagEstimator
	spacing, span core.Primitive
	paths         [2]LogReturns
	seed          *transport.IO
	current       core.Primitive
}

/*
NewLagProfile evaluates the configured estimator at every lag over the same
decoded paths. Spacing is nanoseconds;
span counts steps on either side. Every candidate retains the complete estimator
record and its own support. Index identity is retained before conversion to
seconds, so boundary indices cannot be corrupted by floating-point round trips.
*/
func NewLagProfile(estimator LagEstimator, spacing, span core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewMap(&LagProfile{
			estimator: estimator, spacing: spacing, span: span,
			seed: transport.NewIO(core.From([]core.Primitive{})),
		}),
		transport.NewSpread[core.Primitive](),
	)
}

/* Next consumes one input pair and emits its complete immutable candidate run. */
func (profile *LagProfile) Next(input core.Primitive) core.Primitive {
	result := core.Yield(profile.seed, input,
		func(_ []core.Primitive, fields map[string]core.Primitive) []core.Primitive {
			candidates, err := profile.Search(fields)
			profile.Error(err)
			return candidates
		}, profile)

	if result != nil {
		profile.current = result
	}
	return result
}

/* Search decodes path coordinates once and leaves covariance ownership to the estimator. */
func (profile *LagProfile) Search(fields map[string]core.Primitive) ([]core.Primitive, error) {
	decoder := core.NewDecoder(fields)
	left := core.Decode[[]core.Primitive](decoder, "left")
	right := core.Decode[[]core.Primitive](decoder, "right")

	if err := decoder.Error(); err != nil {
		return nil, err
	}
	spacing, err := transport.Evaluate[float64](profile.spacing, nil)

	if err != nil {
		return nil, err
	}
	span, err := transport.Evaluate[float64](profile.span, nil)

	if err != nil {
		return nil, err
	}
	if err := profile.paths[0].Load(left); err != nil {
		return nil, err
	}

	if err := profile.paths[1].Load(right); err != nil {
		return nil, err
	}
	// Range owns validation of the discrete count, including non-integral counts.
	sequence := transport.NewRange(transport.NewIO(core.From(span*2 + 1)))
	candidates := []core.Primitive{}
	core.Yield(transport.NewIO(core.From(0.0)), sequence, func(held, index float64) float64 {
		candidate, err := profile.Candidate(index, span, spacing)
		profile.Error(err)
		candidates = append(candidates, candidate)
		return held
	}, profile)
	return candidates, profile.Error()
}

/* Candidate applies one exact timestamp offset without copying either path. */
func (profile *LagProfile) Candidate(index, span, spacing float64) (core.Primitive, error) {
	lagIndex := index - span
	lag := int64(lagIndex * spacing)
	estimate, err := profile.estimator.Estimate(&profile.paths[0], &profile.paths[1], lag)

	if err != nil {
		return nil, err
	}
	coefficient, err := core.Field[float64](estimate, "correlation")

	if err != nil {
		return nil, err
	}
	fields := make(map[string]core.Primitive, len(estimate)+4)

	for name, value := range estimate {
		fields[name] = value
	}
	fields["index"], fields["lag_index"] = core.From(index), core.From(lagIndex)
	fields["x"], fields["y"] = core.From(float64(lag)*1e-9), core.From(coefficient)
	return core.From(fields), nil
}

func (profile *LagProfile) Read() any { return core.To[any](profile.current) }
