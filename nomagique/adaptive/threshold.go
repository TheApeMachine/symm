package adaptive

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Threshold composes a moment estimator with a dispersion coefficient.
A source with no dispersion has threshold one.
*/
type Threshold struct {
	*core.PrimitiveError

	moments     core.Primitive
	coefficient core.Primitive
	out         float64
}

func NewThreshold(
	moments core.Primitive,
	coefficient core.Primitive,
) *Threshold {
	return &Threshold{PrimitiveError: core.NewPrimitiveError(), moments: moments,
		coefficient: coefficient,
	}
}

func (threshold *Threshold) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			if threshold.moments != nil {
				if err := threshold.moments.Error(); err != nil {
					threshold.Error(err)
				}
			}

			if threshold.coefficient != nil {
				if err := threshold.coefficient.Error(); err != nil {
					threshold.Error(err)
				}
			}
		}()
		for readingPtr := range threshold.moments.Next(in) {
			current := *(*statistic.MomentReading)(readingPtr)
			coeffValEval := threshold.coefficient
			var coeffVal float64

			for out := range coeffValEval.Next(sequence.NewValues(current.Count).Next(nil)) {
				coeffVal = *(*float64)(out)
			}

			err := coeffValEval.Error()

			if err != nil {
				threshold.Error(err)
				return
			}

			if current.Dispersion > 0 {
				threshold.out = current.Dispersion * coeffVal
			} else {
				threshold.out = 1
			}

			if !yield(unsafe.Pointer(&threshold.out)) {
				return
			}
		}
	}
}
