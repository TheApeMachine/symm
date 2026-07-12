package manifold

import (
	"math"
	"time"
)

/*
EpochScale applies one time-elastic update per complete population epoch. Batch
statistics therefore do not depend on order identity or iteration order.
*/
type EpochScale struct {
	halflife time.Duration
	value    float64
	at       time.Time
	ready    bool
}

func NewEpochScale(halflife time.Duration) *EpochScale {
	return &EpochScale{halflife: halflife}
}

func (scale *EpochScale) Update(value float64, at time.Time) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || at.IsZero() || scale.halflife <= 0 {
		return 0, false
	}

	if !scale.ready {
		scale.value = value
		scale.at = at
		scale.ready = true
		return scale.value, true
	}

	if !at.After(scale.at) {
		return scale.value, true
	}

	alpha := 1 - math.Exp(-math.Ln2*at.Sub(scale.at).Seconds()/scale.halflife.Seconds())
	scale.value += alpha * (value - scale.value)
	scale.at = at

	return scale.value, true
}

func (scale *EpochScale) Value() float64 {
	return scale.value
}
