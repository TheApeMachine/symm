package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
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
	err error
	out FisherReading
}

func NewFisher() core.Primitive {
	return &Fisher{}
}

/*
Compute evaluates the Fisher-z normal approximation directly without iterator overhead.
*/
func (op *Fisher) Compute(sample *FisherSample) FisherReading {
	reading := FisherReading{
		HasSearch: sample.SearchCount >= 1,
	}

	if sample.Support > 3 && math.Abs(sample.Correlation) <= 1 {
		degrees := math.Sqrt(sample.Support - 3)
		z := math.Atanh(sample.Correlation) * degrees
		p := math.Erfc(math.Abs(z) / math.Sqrt2)

		reading.Defined = true
		reading.PValue = p
		reading.Z = z
		reading.StandardError = 1.0 / degrees

		if reading.HasSearch {
			adj := p * sample.SearchCount

			if adj > 1.0 {
				adj = 1.0
			}

			reading.SearchAdjustedPValue = adj
		}
	}

	return reading
}

func (op *Fisher) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*FisherSample)(arriving)
			op.out = op.Compute(sample)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Fisher) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
