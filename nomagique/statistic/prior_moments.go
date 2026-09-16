package statistic

import (
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
func (priorMoments *PriorMoments) age(epoch uint64, memory float64) {
	if epoch <= priorMoments.LastEpoch {
		return
	}

	if memory > 1 {
		gap := float64(epoch - priorMoments.LastEpoch)
		priorMoments.Weight *= math.Exp(gap * math.Log(1-1/memory))
	}
	priorMoments.LastEpoch = epoch
}

/*
summary computes the same reliability-weighted variance and signal authority.
*/
func (priorMoments PriorMoments) summary(memory float64) PriorSummary {
	reading := PriorSummary{Samples: priorMoments.Samples, Pending: priorMoments.Pending, Memory: memory}

	if priorMoments.Weight <= 0 {
		return reading
	}
	reading.Defined, reading.Mean, reading.Support = true, priorMoments.Mean, priorMoments.Support
	reading.EvidenceAuthority = priorMoments.Weight / priorMoments.Support

	if priorMoments.Support <= 1 {
		return reading
	}
	reading.VarianceDefined = true
	reading.Variance = priorMoments.Moment * (priorMoments.Support / (priorMoments.Support - 1))
	reading.Maturity = (priorMoments.Support - 1) / priorMoments.Support
	power := priorMoments.Mean * priorMoments.Mean
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
	*core.PrimitiveError

	moments PriorMoments
	out     PriorSummary
}

/*
NewPriorMoments instantiates the reliability-weighted prior Primitive.
*/
func NewPriorMoments() *PriorEstimator {
	return &PriorEstimator{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next applies one observation or age-only query to the prior recurrence and
hands over the resulting summary.
*/
func (priorEstimator *PriorEstimator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			observation := (*PriorObservation)(arriving)

			if observation.AgeOnly {
				if !priorEstimator.observeQuery(observation) {
					return
				}

				if !yield(unsafe.Pointer(&priorEstimator.out)) {
					return
				}

				continue
			}

			if !priorEstimator.observe(observation) {
				return
			}

			if !yield(unsafe.Pointer(&priorEstimator.out)) {
				return
			}
		}
	}
}

/*
observeQuery ages the causal clock without counting a sample.
*/
func (priorEstimator *PriorEstimator) observeQuery(observation *PriorObservation) bool {
	if !observation.HasEpoch {
		priorEstimator.out = priorEstimator.moments.summary(observation.Memory)
		return true
	}

	priorEstimator.moments.age(observation.Epoch, observation.Memory)
	priorEstimator.out = priorEstimator.moments.summary(observation.Memory)
	return true
}

/*
observe records one completion, aging only through positive authority exactly
as the recurrence requires.
*/
func (priorEstimator *PriorEstimator) observe(observation *PriorObservation) bool {
	if !(observation.Authority >= 0 && observation.Authority <= 1) {
		priorEstimator.Error(fmt.Errorf("%w: prior authority must be in [0, 1]", core.ErrDomain))
		return false
	}
	priorEstimator.moments.Samples++

	if observation.Authority == 0 {
		priorEstimator.out = priorEstimator.moments.summary(observation.Memory)
		return true
	}

	if observation.HasEpoch {
		priorEstimator.moments.age(observation.Epoch, observation.Memory)
	}

	if !observation.HasEpoch && observation.Memory > 1 {
		priorEstimator.moments.Weight *= math.Exp(math.Log(1 - 1/observation.Memory))
	}

	if priorEstimator.moments.Weight == 0 {
		priorEstimator.moments.Mean, priorEstimator.moments.Weight = observation.Value, observation.Authority
		priorEstimator.moments.Support, priorEstimator.moments.Moment = 1, 0
		priorEstimator.out = priorEstimator.moments.summary(observation.Memory)
		return true
	}
	total := priorEstimator.moments.Weight + observation.Authority
	retained, incoming := priorEstimator.moments.Weight/total, observation.Authority/total
	difference := observation.Value - priorEstimator.moments.Mean
	priorEstimator.moments.Support = 1 / (retained*retained/priorEstimator.moments.Support + incoming*incoming)
	priorEstimator.moments.Moment = retained*priorEstimator.moments.Moment + (retained*incoming)*(difference*difference)
	priorEstimator.moments.Mean += incoming * difference
	priorEstimator.moments.Weight = total
	priorEstimator.out = priorEstimator.moments.summary(observation.Memory)
	return true
}
