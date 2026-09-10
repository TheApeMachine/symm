package correlation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
FisherSample is a correlation, its support, and an optional search multiplicity.
A finite-sample overlap count is not thereby made an independent sample size.
*/
type FisherSample struct {
	Correlation float64
	Support     float64
	SearchCount float64
}

/*
FisherReading is the Fisher-z normal approximation. Undefined inputs yield
Defined=false and NaN, never a stale p-value. At ±1 the extended-real limit
yields p=0, not p=1.
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
	core.Base[FisherSample, FisherReading]
	fisher     *equation.Fisher
	bonferroni *equation.Bonferroni[float64]
}

func NewFisher() *Fisher {
	return &Fisher{
		fisher:     equation.NewFisher(),
		bonferroni: equation.NewBonferroni[float64](),
	}
}

func (op *Fisher) Next(
	in iter.Seq[core.Primitive[FisherSample, FisherSample]],
) iter.Seq[core.Primitive[FisherReading, FisherReading]] {
	return func(yield func(core.Primitive[FisherReading, FisherReading]) bool) {
		for arriving := range in {
			reading, err := op.Evaluate(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

func (op *Fisher) Evaluate(sample FisherSample) (FisherReading, error) {
	reading := FisherReading{
		PValue:               math.NaN(),
		Z:                    math.NaN(),
		StandardError:        math.NaN(),
		SearchAdjustedPValue: math.NaN(),
		HasSearch:            sample.SearchCount >= 1,
	}

	if !(sample.Support > 3 && math.Abs(sample.Correlation) <= 1) {
		return reading, nil
	}

	p, err := transport.Evaluate(op.fisher, transport.Values(equation.FisherInput{
		Correlation: sample.Correlation,
		Support:     sample.Support,
	}))

	if err != nil {
		return FisherReading{}, err
	}

	degrees := math.Sqrt(sample.Support - 3)
	reading.Defined = true
	reading.PValue = p
	reading.Z = math.Atanh(sample.Correlation) * degrees
	reading.StandardError = 1 / degrees

	if !reading.HasSearch {
		return reading, nil
	}

	adjusted, err := transport.Evaluate(op.bonferroni, transport.Values(equation.BonferroniInput[float64]{
		P:          p,
		Candidates: sample.SearchCount,
	}))

	if err != nil {
		return FisherReading{}, err
	}

	reading.SearchAdjustedPValue = adjusted
	return reading, nil
}
