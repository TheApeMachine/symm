package statistic

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
PriorEstimator owns the reliability-weighted prior recurrence as a Primitive.
*/
type PriorEstimator struct {
	*core.PrimitiveError
	moments PriorMoments
	out     PriorSummary
}

/*
NewPriorEstimator instantiates the reliability-weighted prior Primitive.
*/
func NewPriorEstimator() *PriorEstimator {
	return &PriorEstimator{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next applies one observation or age-only query to the prior recurrence and
hands over the resulting summary.
*/
func (op *PriorEstimator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			observation := (*PriorObservation)(arriving)

			if observation.AgeOnly {
				if !op.observeQuery(observation) {
					return
				}

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			if !op.observe(observation) {
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
observeQuery ages the causal clock without counting a sample.
*/
func (op *PriorEstimator) observeQuery(observation *PriorObservation) bool {
	if !observation.HasEpoch {
		op.out = op.moments.summary(observation.Memory)
		return true
	}

	op.moments.age(observation.Epoch, observation.Memory)
	op.out = op.moments.summary(observation.Memory)
	return true
}

/*
observe records one completion, aging only through positive authority exactly
as the recurrence requires.
*/
func (op *PriorEstimator) observe(observation *PriorObservation) bool {
	if !(observation.Authority >= 0 && observation.Authority <= 1) {
		op.Error(fmt.Errorf("%w: prior authority must be in [0, 1]", core.ErrDomain))
		return false
	}
	op.moments.Samples++

	if observation.Authority == 0 {
		op.out = op.moments.summary(observation.Memory)
		return true
	}

	if observation.HasEpoch {
		op.moments.age(observation.Epoch, observation.Memory)
	}

	if !observation.HasEpoch && observation.Memory > 1 {
		op.moments.Weight *= math.Exp(math.Log(1 - 1/observation.Memory))
	}

	if op.moments.Weight == 0 {
		op.moments.Mean, op.moments.Weight = observation.Value, observation.Authority
		op.moments.Support, op.moments.Moment = 1, 0
		op.out = op.moments.summary(observation.Memory)
		return true
	}

	total := op.moments.Weight + observation.Authority
	retained, incoming := op.moments.Weight/total, observation.Authority/total
	difference := observation.Value - op.moments.Mean
	op.moments.Support = 1 / (retained*retained/op.moments.Support + incoming*incoming)
	op.moments.Moment = retained*op.moments.Moment + (retained*incoming)*(difference*difference)
	op.moments.Mean += incoming * difference
	op.moments.Weight = total
	op.out = op.moments.summary(observation.Memory)
	return true
}
