package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
FisherSample is a correlation, its support, and an optional search multiplicity.
*/
type FisherSample struct {
	Correlation float64
	Support     float64
	SearchCount float64
}

/*
FisherReading is the Fisher-z normal approximation. Undefined inputs yield
Defined=false and NaN.
*/
type FisherReading struct {
	Defined              bool
	PValue               float64
	Z                    float64
	StandardError        float64
	SearchAdjustedPValue float64
	HasSearch            bool
}

/*
Fisher owns that approximation.
*/
type Fisher struct {
	*core.PrimitiveError

	support   core.Primitive
	threshold core.Primitive
}

func NewFisher(primitives ...core.Primitive) *Fisher {
	fisher := &Fisher{
		PrimitiveError: core.NewPrimitiveError(),
		support:        adaptive.NewBaseline(adaptive.NewWindow()),
		threshold:      adaptive.NewThreshold(statistic.NewEstimator(), calculus.NewSqrt()),
	}

	if len(primitives) > 0 && primitives[0] != nil {
		fisher.support = primitives[0]
	}

	if len(primitives) > 1 && primitives[1] != nil {
		fisher.threshold = primitives[1]
	}

	return fisher
}

func (fisher *Fisher) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*FisherSample)(arriving)
			reading := FisherReading{
				HasSearch: sample.SearchCount >= core.Unit,
			}

			supportVal := sample.Support
			thresholdVal := core.Unit

			if sample.Support > 0 && fisher.support != nil {
				for out := range fisher.support.Next(sequence.NewOne(unsafe.Pointer(&sample.Support)).Next(nil)) {
					supportReading := (*adaptive.BaselineReading)(out)
					supportVal = supportReading.Baseline
				}
			}

			if sample.Support > 0 && fisher.threshold != nil {
				for out := range fisher.threshold.Next(sequence.NewOne(unsafe.Pointer(&sample.Support)).Next(nil)) {
					thresholdVal = *(*float64)(out)
				}
			}

			effectiveSupport := supportVal - thresholdVal

			if sample.Support > thresholdVal && effectiveSupport > 0 && math.Abs(sample.Correlation) <= core.Unit {
				degrees := math.Sqrt(effectiveSupport)
				z := math.Atanh(sample.Correlation) * degrees
				p := math.Erfc(math.Abs(z) / math.Sqrt2)

				reading.Defined = true
				reading.PValue = p
				reading.Z = z
				reading.StandardError = core.Unit / degrees

				if reading.HasSearch {
					adj := p * sample.SearchCount

					if adj > core.Unit {
						adj = core.Unit
					}

					reading.SearchAdjustedPValue = adj
				}
			}

			if !yield(unsafe.Pointer(&reading)) {
				return
			}
		}
	}
}
