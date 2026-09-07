package correlation

import (
	"math"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Dependence owns the typed path diagnostics surrounding an opaque estimator. */
type Dependence struct {
	core.PrimitiveError
	paths   [2]equation.LogReturns
	seed    *transport.IO
	current core.Primitive
}

/*
NewDependence preserves the configured estimator and adds support, path duration,
energy-rate and definedness diagnostics. The estimator executes once per pair.
Finite-sample correlation remains unclipped; zero-energy estimates are undefined.
*/
func NewDependence(estimator core.Primitive) core.Primitive {
	return transport.NewPipe(
		store.NewRecord(transport.NewPipe(), transport.NewPipe(estimator, store.NewKey("estimate"))),
		transport.NewMap(&Dependence{seed: transport.NewIO(core.From(map[string]core.Primitive{}))}),
	)
}

/* Next computes path diagnostics once, with no graph construction per return. */
func (dependence *Dependence) Next(input core.Primitive) core.Primitive {
	result := core.Yield(dependence.seed, input,
		func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
			for index, name := range [2]string{"left", "right"} {
				observations, err := core.Field[[]core.Primitive](fields, name)

				if err != nil {
					dependence.Error(err)
					return nil
				}

				if err := dependence.paths[index].Load(observations); err != nil {
					dependence.Error(err)
					return nil
				}
			}
			estimate, err := core.Field[map[string]core.Primitive](fields, "estimate")

			if err != nil {
				dependence.Error(err)
				return nil
			}
			return dependence.Summarize(estimate)
		}, dependence)

	if result != nil {
		dependence.current = result
	}
	return result
}

/* Summarize preserves estimator fields and derives the existing reporting facts. */
func (dependence *Dependence) Summarize(estimate map[string]core.Primitive) map[string]core.Primitive {
	decoder := core.NewDecoder(estimate)
	support := core.Decode[float64](decoder, "support")
	leftEnergy := core.Decode[float64](decoder, "left_energy")
	rightEnergy := core.Decode[float64](decoder, "right_energy")
	coefficient := core.Decode[float64](decoder, "correlation")

	if err := decoder.Error(); err != nil {
		dependence.Error(err)
		return nil
	}
	fields := make(map[string]core.Primitive, len(estimate)+8)

	for name, value := range estimate {
		fields[name] = value
	}
	left, right := &dependence.paths[0], &dependence.paths[1]
	shared, density := 0.0, 0.0

	if len(left.Intervals) > 0 && len(right.Intervals) > 0 {
		shared = max(0, float64(min(left.Through, right.Through)-max(left.From, right.From))/float64(time.Second))
	}

	if shared > 0 {
		density = support / shared
	}
	fields["left_returns"] = core.From(float64(len(left.Intervals)))
	fields["right_returns"] = core.From(float64(len(right.Intervals)))
	fields["left_energy_rate"] = core.From(left.MedianEnergyRate())
	fields["right_energy_rate"] = core.From(right.MedianEnergyRate())
	fields["defined"] = core.From(support > 0 && leftEnergy > 0 && rightEnergy > 0 && !math.IsNaN(coefficient) && !math.IsInf(coefficient, 0))
	fields["shared_time"], fields["overlap_density"] = core.From(shared), core.From(density)
	return fields
}

func (dependence *Dependence) Read() any { return core.To[any](dependence.current) }
