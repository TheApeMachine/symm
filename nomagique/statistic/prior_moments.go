package statistic

import (
	"math"
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
