package probability

import (
	"errors"
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
func NewGeomean() core.Primitive {
	return NewGeometricMean()
}

/*
ShannonAmbiguity folds a run of non-negative weights into their running
normalized Shannon entropy: the entropy of the weights normalized by their
total, divided by the entropy of an equal-mass distribution over the same
count. A one-member run has zero ambiguity by definition.
*/
type ShannonAmbiguity struct {
	err    error
	values []float64
	total  float64
}

/*
NewShannonAmbiguity instantiates the normalized-entropy reduction Primitive.
*/
func NewShannonAmbiguity() core.Primitive {
	return &ShannonAmbiguity{}
}

/*
Next folds every arriving weight into the run and rewrites the arrival with
the running normalized entropy of everything seen so far.
*/
func (op *ShannonAmbiguity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			op.values = append(op.values, val)
			op.total += val
			*(*float64)(arriving) = ambiguity(op.values, op.total)

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *ShannonAmbiguity) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
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
