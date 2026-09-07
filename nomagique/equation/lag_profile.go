package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* LagProfile owns the configured estimator and exact discrete search coordinates. */
type LagProfile struct {
	core.PrimitiveError
	estimator, spacing, span core.Primitive
	seed                     *transport.IO
	current                  core.Primitive
}

/*
NewLagProfile evaluates the opaque estimator at every lag. Spacing is nanoseconds;
span counts steps on either side. Every candidate retains the complete estimator
record and its own support. Index identity is retained before conversion to
seconds, so boundary indices cannot be corrupted by floating-point round trips.
*/
func NewLagProfile(estimator, spacing, span core.Primitive) core.Primitive {
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
	times := make([]int64, len(left))
	values := make([]core.Primitive, len(left))

	for index, observation := range left {
		record := core.To[map[string]core.Primitive](observation)

		if err := observation.Error(); err != nil {
			return nil, err
		}
		point := core.NewDecoder(record)
		times[index] = core.Decode[int64](point, "at")
		values[index] = core.From(core.Decode[float64](point, "value"))

		if err := point.Error(); err != nil {
			return nil, err
		}
	}
	// Range owns validation of the discrete count, including non-integral counts.
	sequence := transport.NewRange(transport.NewIO(core.From(span*2 + 1)))
	candidates := []core.Primitive{}
	core.Yield(transport.NewIO(core.From(0.0)), sequence, func(held, index float64) float64 {
		candidate, err := profile.Candidate(times, values, right, index, span, spacing)
		profile.Error(err)
		candidates = append(candidates, candidate)
		return held
	}, profile)
	return candidates, profile.Error()
}

/* Candidate shifts timestamps without rebuilding a scalar processing graph. */
func (profile *LagProfile) Candidate(
	times []int64, values, right []core.Primitive, index, span, spacing float64,
) (core.Primitive, error) {
	lagIndex := index - span
	lag := int64(lagIndex * spacing)
	shifted := make([]core.Primitive, len(times))

	for position, at := range times {
		shifted[position] = core.From(map[string]core.Primitive{
			"at": core.From(at + lag), "value": values[position],
		})
	}
	estimate, err := transport.Evaluate[map[string]core.Primitive](profile.estimator,
		core.Record(map[string]any{"left": shifted, "right": right}))

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
