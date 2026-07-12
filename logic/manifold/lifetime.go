package manifold

import (
	"sort"
	"time"
)

type lifetimeObservation struct {
	duration  time.Duration
	completed bool
}

type lifetimeEstimate struct {
	duration time.Duration
	cdf      float64
}

/*
LifetimeEstimator owns a bounded chronological sample and evaluates the
Kaplan-Meier empirical lifetime CDF with right-censored observations.
*/
type LifetimeEstimator struct {
	capacity     int
	observations []lifetimeObservation
	ordered      []lifetimeObservation
	estimates    []lifetimeEstimate
	dirty        bool
}

func NewLifetimeEstimator(capacity int) *LifetimeEstimator {
	allocation := max(capacity, 0)

	return &LifetimeEstimator{
		capacity:     capacity,
		observations: make([]lifetimeObservation, 0, allocation),
		ordered:      make([]lifetimeObservation, 0, allocation),
		estimates:    make([]lifetimeEstimate, 0, allocation),
	}
}

func (estimator *LifetimeEstimator) Ready() bool {
	return estimator != nil && len(estimator.observations) > 0
}

func (estimator *LifetimeEstimator) RecordCompleted(lifetime time.Duration) {
	estimator.record(lifetime, true)
}

func (estimator *LifetimeEstimator) Censor(age time.Duration) {
	estimator.record(age, false)
}

/*
CDF returns the estimated probability that an order lifetime is at or below age.
*/
func (estimator *LifetimeEstimator) CDF(age time.Duration) float64 {
	if !estimator.Ready() || age < 0 {
		return 0
	}

	estimator.prepare()
	index := sort.Search(len(estimator.estimates), func(index int) bool {
		return estimator.estimates[index].duration > age
	})

	if index == 0 {
		return 0
	}

	return estimator.estimates[index-1].cdf
}

/*
prepare rebuilds one exact Kaplan-Meier curve from every chronological
observation accumulated before the current epoch. Records mark the curve dirty,
so a CDF query can never read an estimate from an older sample.
*/
func (estimator *LifetimeEstimator) prepare() {
	if !estimator.dirty {
		return
	}

	estimator.ordered = append(estimator.ordered[:0], estimator.observations...)
	sort.Slice(estimator.ordered, func(left, right int) bool {
		return estimator.ordered[left].duration < estimator.ordered[right].duration
	})
	estimator.estimates = estimator.estimates[:0]
	atRisk := len(estimator.ordered)
	survival := 1.0

	for start := 0; start < len(estimator.ordered); {
		duration := estimator.ordered[start].duration
		end := start
		completed := 0

		for end < len(estimator.ordered) && estimator.ordered[end].duration == duration {
			if estimator.ordered[end].completed {
				completed++
			}

			end++
		}

		if completed > 0 && atRisk > 0 {
			survival *= 1 - float64(completed)/float64(atRisk)
		}

		estimator.estimates = append(estimator.estimates, lifetimeEstimate{
			duration: duration,
			cdf:      1 - survival,
		})
		atRisk -= end - start
		start = end
	}

	estimator.dirty = false
}

func (estimator *LifetimeEstimator) SurvivalFraction(age time.Duration) float64 {
	return 1 - estimator.CDF(age)
}

func (estimator *LifetimeEstimator) record(duration time.Duration, completed bool) {
	if estimator == nil || estimator.capacity <= 0 || duration < 0 {
		return
	}

	estimator.observations = append(estimator.observations, lifetimeObservation{
		duration:  duration,
		completed: completed,
	})
	estimator.dirty = true

	if len(estimator.observations) > estimator.capacity {
		estimator.observations = estimator.observations[len(estimator.observations)-estimator.capacity:]
	}
}
