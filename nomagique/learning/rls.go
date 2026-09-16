package learning

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
Sample is one feature vector and an optional target. A features-only query
never trains, so prediction and update share one arrival path: the reading
each arrival yields is the prequential prediction prior to that sample's
optional update.
*/
type Sample struct {
	Features []float64
	Target   float64
	Observed bool
}

/*
RLS supplies an affine intercept and the zero-mean diagonal coefficient prior.
Prediction is prior to the optional target's update, as owned by SquareRootRLS.
*/
type RLS struct {
	*core.PrimitiveError

	dimension int
	lambda    float64
	learner   core.Primitive
	out       algo.Reading
}

/*
NewRLS creates an RLS primitive over the given feature dimension, coefficient
prior variance, and forgetting factor.
*/
func NewRLS(dimension int, variance, lambda float64) *RLS {
	return &RLS{PrimitiveError: core.NewPrimitiveError(), dimension: dimension,
		lambda:  lambda,
		learner: algo.NewSquareRootRLS(variance),
	}
}

func (rls *RLS) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*Sample)(arriving)

			if rls.dimension <= 0 || len(sample.Features) != rls.dimension {
				rls.Error(fmt.Errorf(
					"%w: RLS expected %d features, received %d",
					core.ErrShape,
					rls.dimension,
					len(sample.Features),
				))
				return
			}

			design := make([]float64, len(sample.Features)+1)
			design[0] = 1
			copy(design[1:], sample.Features)

			query := algo.Query{
				Design:   design,
				Target:   sample.Target,
				Observed: sample.Observed,
				Lambda:   rls.lambda,
			}

			for out := range rls.learner.Next(sequence.NewValues(query).Next(nil)) {
				rls.out = *(*algo.Reading)(out)
			}

			if err := rls.learner.Error(); err != nil {
				rls.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&rls.out)) {
				return
			}
		}
	}
}
