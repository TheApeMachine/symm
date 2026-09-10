package correlation

import (
	"iter"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
PathReading is one accepted, restated, or rejected timestamped observation
together with the retained history.
*/
type PathReading struct {
	equation.Price
	Observations []equation.Price
	PriorCount   float64
	Count        float64
	Accepted     bool
	Restated     bool
	HasSpan      bool
	From         int64
	To           int64
}

/*
Path owns timestamp acceptance and the retained observation sequence.
Equal timestamps restate the last observation; regressions return
Accepted=false without editing the path. Optional collection-to-collection
retention remains caller-configured. Emitted observation slices remain
immutable across later updates.
*/
type Path struct {
	core.Base[equation.Price, PathReading]
	retention    core.Primitive[[]equation.Price, []equation.Price]
	observations []equation.Price
}

func NewPath(retention ...core.Primitive[[]equation.Price, []equation.Price]) *Path {
	path := &Path{}

	if len(retention) > 0 {
		path.retention = retention[0]
	}

	return path
}

func (op *Path) Next(
	in iter.Seq[core.Primitive[equation.Price, equation.Price]],
) iter.Seq[core.Primitive[PathReading, PathReading]] {
	return func(yield func(core.Primitive[PathReading, PathReading]) bool) {
		for arriving := range in {
			reading, err := op.Update(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

/*
Update appends outside previously emitted slice lengths. Restatements copy the
slice before replacing a visible observation; retention consumes immutable input.
*/
func (op *Path) Update(sample equation.Price) (PathReading, error) {
	priorCount := len(op.observations)
	last := int64(0)

	if priorCount > 0 {
		last = op.observations[priorCount-1].At
	}

	accepted := priorCount == 0 || sample.At >= last
	restated := priorCount > 0 && sample.At == last

	if accepted {
		observations := op.observations

		if restated {
			observations = slices.Clone(observations)
			observations[priorCount-1] = sample
		}

		if !restated {
			observations = append(observations, sample)
		}

		if op.retention != nil {
			retained, err := transport.Evaluate(op.retention, transport.Values(observations))

			if err != nil {
				return PathReading{}, err
			}

			observations = retained
		}

		op.observations = observations
	}

	from, through := int64(0), int64(0)

	if len(op.observations) > 0 {
		from = op.observations[0].At
		through = op.observations[len(op.observations)-1].At
	}

	return PathReading{
		Price:        sample,
		Observations: op.observations[:len(op.observations):len(op.observations)],
		PriorCount:   float64(priorCount),
		Count:        float64(len(op.observations)),
		Accepted:     accepted,
		Restated:     restated,
		HasSpan:      len(op.observations) != 0,
		From:         from,
		To:           through,
	}, nil
}
