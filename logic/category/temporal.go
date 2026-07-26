package category

import (
	"math"
	"time"
)

/*
evidenceClock is the temporal envelope of a category's supporting measurements.
Horizon is the max producer horizon among those rows so staleness is relative
to the estimator's own scale, not a wall-clock constant.
*/
type evidenceClock struct {
	from    time.Time
	through time.Time
	horizon time.Duration
	mass    float64
	ok      bool
}

/*
alignable reports whether two evidence clocks can be compared temporally.
Disjoint intervals with no horizon coverage are IncomparableWith.
*/
func alignable(left, right evidenceClock) bool {
	if !left.ok || !right.ok {
		return false
	}

	if !left.through.Before(right.from) && !right.through.Before(left.from) {
		return true
	}

	gap := right.from.Sub(left.through)

	if gap < 0 {
		gap = left.from.Sub(right.through)
	}

	cover := left.horizon

	if right.horizon > cover {
		cover = right.horizon
	}

	return cover > 0 && gap <= cover
}

/*
staleMass returns mass when left's evidence is stale relative to right: left's
latest sample is older than right's by more than left's own horizon.
*/
func staleMass(left, right evidenceClock, leftStrength, rightStrength float64) float64 {
	if !left.ok || !right.ok || left.horizon <= 0 {
		return 0
	}

	if !right.through.After(left.through) {
		return 0
	}

	if right.through.Sub(left.through) <= left.horizon {
		return 0
	}

	return math.Sqrt(leftStrength * rightStrength)
}

/*
leadMass returns mass when left's evidence envelope precedes right's on an
alignable clock. Contemporaneous envelopes yield zero.
*/
func leadMass(left, right evidenceClock, leftStrength, rightStrength float64) float64 {
	if !alignable(left, right) {
		return 0
	}

	if !left.through.Before(right.from) && !right.through.Before(left.from) {
		if left.from.Equal(right.from) {
			return 0
		}

		if left.from.Before(right.from) {
			return math.Sqrt(leftStrength * rightStrength)
		}

		return 0
	}

	if left.through.Before(right.from) || left.from.Before(right.from) {
		return math.Sqrt(leftStrength * rightStrength)
	}

	return 0
}
