package prior

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
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
	*core.PrimitiveError

	moments  core.Primitive
	memory   float64
	out      Reading
	replayed Reading
}

func New(memory float64) *Primitive {
	return &Primitive{PrimitiveError: core.NewPrimitiveError(), moments: statistic.NewPriorMoments(), memory: memory}
}

func (primitive *Primitive) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			observation := (*Observation)(arriving)

			if !primitive.step(observation) {
				return
			}

			if !yield(unsafe.Pointer(&primitive.out)) {
				return
			}
		}
	}
}

/*
step drives the canonical prior Primitive with one observation, or replays the
unchanged reading for a query that neither observes nor ages.
*/
func (primitive *Primitive) step(observation *Observation) bool {
	if observation.HasValue {
		request := statistic.PriorObservation{
			Value:     observation.Value,
			Authority: observation.Authority,
			Memory:    primitive.memory,
			Epoch:     observation.Epoch,
			HasEpoch:  observation.HasEpoch,
		}

		return primitive.deliver(request)
	}

	if observation.HasEpoch {
		request := statistic.PriorObservation{
			Memory:   primitive.memory,
			Epoch:    observation.Epoch,
			HasEpoch: true,
			AgeOnly:  true,
		}

		return primitive.deliver(request)
	}

	primitive.out = primitive.replayed
	return true
}

/*
deliver folds one request through the canonical prior Primitive.
*/
func (primitive *Primitive) deliver(request statistic.PriorObservation) bool {
	for summary := range primitive.moments.Next(sequence.NewOne(unsafe.Pointer(&request)).Next(nil)) {
		primitive.out = fromSummary(*(*statistic.PriorSummary)(summary))
	}

	if err := primitive.moments.Error(); err != nil {
		primitive.Error(err)
		return false
	}

	primitive.replayed = primitive.out
	return true
}
