package statistic

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
PriorMoments owns normalized reliability-weighted moments and their causal
clock as pure state. Its recurrence lives in the PriorEstimator Primitive.
*/
type PriorMoments struct {
	Samples, Pending, LastEpoch   uint64
	Mean, Weight, Support, Moment float64
}

/*
PriorObservation is one arrival for the PriorEstimator Primitive: a completion
(Value with its Authority under a Memory), or an age-only query (AgeOnly with
an Epoch) that discounts evidence without counting a sample.
*/
type PriorObservation struct {
	Value     float64
	Authority float64
	Memory    float64
	Epoch     uint64
	HasEpoch  bool
	AgeOnly   bool
}

/*
PriorSummary separates completion, support, dispersion and retained authority.
*/
type PriorSummary struct {
	Samples, Pending                     uint64
	Defined, VarianceDefined             bool
	Mean, Variance, Support, Maturity    float64
	EvidenceAuthority, Authority, Memory float64
}

/*
age discounts total weight; normalized moment and support are scale invariant.
*/
func (moments *PriorMoments) age(epoch uint64, memory float64) {
	if epoch <= moments.LastEpoch {
		return
	}

	if memory > 1 {
		gap := float64(epoch - moments.LastEpoch)
		moments.Weight *= math.Exp(gap * math.Log(1-1/memory))
	}
	moments.LastEpoch = epoch
}

/*
summary computes the same reliability-weighted variance and signal authority.
*/
func (moments PriorMoments) summary(memory float64) PriorSummary {
	reading := PriorSummary{Samples: moments.Samples, Pending: moments.Pending, Memory: memory}

	if moments.Weight <= 0 {
		return reading
	}
	reading.Defined, reading.Mean, reading.Support = true, moments.Mean, moments.Support
	reading.EvidenceAuthority = moments.Weight / moments.Support

	if moments.Support <= 1 {
		return reading
	}
	reading.VarianceDefined = true
	reading.Variance = moments.Moment * (moments.Support / (moments.Support - 1))
	reading.Maturity = (moments.Support - 1) / moments.Support
	power := moments.Mean * moments.Mean
	totalPower := power + reading.Variance

	if totalPower > 0 {
		reading.Authority = (reading.Maturity * reading.EvidenceAuthority) * (power / totalPower)
	}
	return reading
}

/*
PriorEstimator owns the reliability-weighted prior recurrence as a Primitive.
*/
type PriorEstimator struct {
	err     error
	moments PriorMoments
	out     PriorSummary
}

/*
NewPriorMoments instantiates the reliability-weighted prior Primitive.
*/
func NewPriorMoments() core.Primitive {
	return &PriorEstimator{}
}

/*
Next applies one observation or age-only query to the prior recurrence and
hands over the resulting summary.
*/
func (op *PriorEstimator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
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
		op.err = fmt.Errorf("%w: prior authority must be in [0, 1]", core.ErrDomain)
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

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *PriorEstimator) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
