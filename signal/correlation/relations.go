package correlation

import (
	"iter"
	"math"
	"sync"
	"time"

	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
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
	mutex sync.RWMutex
	pairs map[[2]string]Relation
}

func (relations *Relations) observe(left, right string, pair *nmcorrelation.Hayashi, fisher *nmcorrelation.Fisher) {
	if right < left {
		left, right = right, left
	}

	_, leftAt, _ := pair.Left.Span()
	_, rightAt, _ := pair.Right.Span()
	value := Relation{
		Left: left, Right: right,
		Signed: float64(pair.Correlation()), Absolute: math.Abs(float64(pair.Correlation())),
		Support: float64(pair.Support()), Defined: pair.Ready(),
		FisherDefined: fisher.Ready(), PValue: float64(fisher.PValue()),
		StandardError: float64(fisher.StandardError()), At: time.Unix(0, min(leftAt, rightAt)),
	}
	relations.mutex.Lock()
	defer relations.mutex.Unlock()

	if relations.pairs == nil {
		relations.pairs = make(map[[2]string]Relation)
	}

	relations.pairs[[2]string{left, right}] = value
}

func (relations *Relations) All() iter.Seq[Relation] {
	return func(yield func(Relation) bool) {
		relations.mutex.RLock()
		defer relations.mutex.RUnlock()

		for _, relation := range relations.pairs {
			if !yield(relation) {
				return
			}
		}
	}
}
