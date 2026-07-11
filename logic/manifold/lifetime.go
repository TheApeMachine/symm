package manifold

import (
	"sort"
	"time"
)

/*
LifetimeEstimator tracks completed order lifetimes and right-censored
active ages for empirical survival coordinates.
*/
type LifetimeEstimator struct {
	completed []time.Duration
	censored  []time.Duration
}

func NewLifetimeEstimator() *LifetimeEstimator {
	return &LifetimeEstimator{
		completed: make([]time.Duration, 0, 256),
		censored:  make([]time.Duration, 0, 256),
	}
}

func (estimator *LifetimeEstimator) Ready() bool {
	return len(estimator.completed)+len(estimator.censored) > 0
}

func (estimator *LifetimeEstimator) RecordCompleted(lifetime time.Duration) {
	if lifetime < 0 {
		return
	}

	estimator.completed = append(estimator.completed, lifetime)
	estimator.trim()
}

func (estimator *LifetimeEstimator) Censor(age time.Duration) {
	if age < 0 {
		return
	}

	estimator.censored = append(estimator.censored, age)
	estimator.trim()
}

func (estimator *LifetimeEstimator) SurvivalFraction(age time.Duration) float64 {
	observations := len(estimator.completed) + len(estimator.censored)

	if observations == 0 {
		return 0
	}

	ageSeconds := age.Seconds()
	survived := 0

	for _, lifetime := range estimator.completed {
		if lifetime.Seconds() > ageSeconds {
			survived++
		}
	}

	for _, censoredAge := range estimator.censored {
		if censoredAge.Seconds() > ageSeconds {
			survived++
		}
	}

	return float64(survived) / float64(observations)
}

func (estimator *LifetimeEstimator) trim() {
	const capacity = 4096

	if len(estimator.completed) <= capacity && len(estimator.censored) <= capacity {
		return
	}

	if len(estimator.completed) > capacity {
		sort.Slice(estimator.completed, func(left, right int) bool {
			return estimator.completed[left] < estimator.completed[right]
		})
		estimator.completed = append(
			[]time.Duration(nil),
			estimator.completed[len(estimator.completed)-capacity/2:]...,
		)
	}

	if len(estimator.censored) > capacity {
		sort.Slice(estimator.censored, func(left, right int) bool {
			return estimator.censored[left] < estimator.censored[right]
		})
		estimator.censored = append(
			[]time.Duration(nil),
			estimator.censored[len(estimator.censored)-capacity/2:]...,
		)
	}
}
