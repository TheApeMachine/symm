package correlation

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
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
Relations retains measured pair measurements. It stores data and answers
nothing else; consumers query it like any other store.
*/
type Relations struct {
	*core.PrimitiveError

	pairs map[[2]string]Relation
}

func NewRelations() *Relations {
	return &Relations{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next receives *Relation payloads and retains each under its ordered symbol
pair.
*/
func (relations *Relations) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			relation := (*Relation)(arriving)

			if relations.pairs == nil {
				relations.pairs = make(map[[2]string]Relation)
			}

			relations.pairs[[2]string{relation.Left, relation.Right}] = *relation

			if !yield(arriving) {
				return
			}
		}
	}
}
