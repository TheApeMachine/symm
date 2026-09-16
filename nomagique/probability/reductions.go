package probability

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Geomean folds a run of values into their running geometric mean,
exp(mean(log x)), handing the aggregate over after every arrival. It is the
canonical composition of the GeometricMean recurrence.
*/
func NewGeomean() *GeometricMean {
	return NewGeometricMean()
}

/*
ShannonAmbiguity folds a run of non-negative weights into their running
normalized Shannon entropy: the entropy of the weights normalized by their
total, divided by the entropy of an equal-mass distribution over the same
count. A one-member run has zero ambiguity by definition.
*/
type ShannonAmbiguity struct {
	*core.PrimitiveError

	values []float64
	total  float64
}

/*
NewShannonAmbiguity instantiates the normalized-entropy reduction Primitive.
*/
func NewShannonAmbiguity() *ShannonAmbiguity {
	return &ShannonAmbiguity{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next folds every arriving weight into the run and rewrites the arrival with
the running normalized entropy of everything seen so far.
*/
func (shannonAmbiguity *ShannonAmbiguity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			shannonAmbiguity.values = append(shannonAmbiguity.values, val)
			shannonAmbiguity.total += val
			*(*float64)(arriving) = ambiguity(shannonAmbiguity.values, shannonAmbiguity.total)

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
ambiguity computes the normalized Shannon entropy of the values against their
total, with a one-member or zero-total distribution defined as zero.
*/
func ambiguity(values []float64, total float64) float64 {
	if len(values) < 2 || total == 0 {
		return 0
	}

	entropy := 0.0

	for _, val := range values {
		p := val / total

		if p > 0 {
			entropy -= p * math.Log(p)
		}
	}

	return entropy / math.Log(float64(len(values)))
}
