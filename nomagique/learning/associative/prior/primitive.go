package prior

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
)

/*
Observation consumes optional value/authority and epoch fields. Queries age
evidence without replaying the previous value.
*/
type Observation struct {
	Value     float64
	Authority float64
	HasValue  bool
	Epoch     uint64
	HasEpoch  bool
}

/*
Primitive binds configured memory to the canonical numeric prior recurrence.
*/
type Primitive struct {
	core.Base[Observation, Reading]
	moments equation.PriorMoments
	memory  float64
}

func New(memory float64) *Primitive {
	return &Primitive{memory: memory}
}

func (op *Primitive) Next(
	in iter.Seq[core.Primitive[Observation, Observation]],
) iter.Seq[core.Primitive[Reading, Reading]] {
	return func(yield func(core.Primitive[Reading, Reading]) bool) {
		for arriving := range in {
			reading, err := op.Transition(arriving.Read())

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

func (op *Primitive) Transition(observation Observation) (Reading, error) {
	if observation.HasValue {
		var epoch []uint64

		if observation.HasEpoch {
			epoch = []uint64{observation.Epoch}
		}

		if err := op.moments.Observe(observation.Value, observation.Authority, op.memory, epoch...); err != nil {
			return Reading{}, err
		}

		return fromSummary(op.moments.Summary(op.memory)), nil
	}

	if observation.HasEpoch {
		op.moments.Age(observation.Epoch, op.memory)
	}

	return fromSummary(op.moments.Summary(op.memory)), nil
}
