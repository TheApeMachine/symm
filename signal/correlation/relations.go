package correlation

import (
	"iter"
	"math"
	"time"
)

/*
Relation is the measured pair before cohort folding. Support counts overlapping
return pairs, not independent samples. EffectiveSupport and Authority are explicitly unavailable (nil):
the Hayashi estimator does not estimate independence-adjusted sample size.
FisherDefined and PValue retain the existing independent-return approximation;
consumers must not mistake that approximation for calibrated authority.
At is the older endpoint of the two paths, so stale peers remain visible.
*/
type Relation struct {
	EffectiveSupport, Authority *float64
	Left, Right                 string
	Signed, Absolute, Support   float64
	PValue, StandardError       float64
	Defined, FisherDefined      bool
	At                          time.Time
}

/*
Relations owns retained pair measurements outside ordinary Grid telemetry.
All exposes values under a read lock; consumers must not retain the lock while
performing an embedding. The zero value is ready for use.
*/
type Relations struct {
	pairs map[[2]string]Relation
}

// observe projects a checked pair record. Undefined Fisher fields remain
// explicitly unavailable; NaN is not serialized as though it were a p-value.
func (relations *Relations) observe(left, right string, pair pairResult, leftAt, rightAt int64) error {
	if right < left {
		left, right = right, left
	}

	value := Relation{Left: left, Right: right, Support: pair.dependence.Support, Defined: pair.dependence.Defined, At: time.Unix(0, min(leftAt, rightAt))}

	if value.Defined {
		value.Signed = pair.dependence.Correlation
		value.Absolute = math.Abs(value.Signed)
	}

	value.FisherDefined = pair.fisher.Defined

	if value.FisherDefined {
		value.PValue = pair.fisher.PValue
		value.StandardError = pair.fisher.StandardError
	}

	if relations.pairs == nil {
		relations.pairs = make(map[[2]string]Relation)
	}
	relations.pairs[[2]string{left, right}] = value
	return nil
}

func (relations *Relations) All() iter.Seq[Relation] {
	return func(yield func(Relation) bool) {
		for _, relation := range relations.pairs {
			if !yield(relation) {
				return
			}
		}
	}
}
