package prior

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
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
	err      error
	moments  core.Primitive
	memory   float64
	out      Reading
	replayed Reading
}

func New(memory float64) core.Primitive {
	return &Primitive{moments: statistic.NewPriorMoments(), memory: memory}
}

func (op *Primitive) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			observation := (*Observation)(arriving)

			if !op.step(observation) {
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
step drives the canonical prior Primitive with one observation, or replays the
unchanged reading for a query that neither observes nor ages.
*/
func (op *Primitive) step(observation *Observation) bool {
	if observation.HasValue {
		request := statistic.PriorObservation{
			Value:     observation.Value,
			Authority: observation.Authority,
			Memory:    op.memory,
			Epoch:     observation.Epoch,
			HasEpoch:  observation.HasEpoch,
		}

		return op.deliver(request)
	}

	if observation.HasEpoch {
		request := statistic.PriorObservation{
			Memory:   op.memory,
			Epoch:    observation.Epoch,
			HasEpoch: true,
			AgeOnly:  true,
		}

		return op.deliver(request)
	}

	op.out = op.replayed
	return true
}

/*
deliver folds one request through the canonical prior Primitive.
*/
func (op *Primitive) deliver(request statistic.PriorObservation) bool {
	for summary := range op.moments.Next(transport.NewOne(unsafe.Pointer(&request)).Next(nil)) {
		op.out = fromSummary(*(*statistic.PriorSummary)(summary))
	}

	if err := op.moments.Error(); err != nil {
		op.err = errors.Join(op.err, err)
		return false
	}

	op.replayed = op.out
	return true
}

func (op *Primitive) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
