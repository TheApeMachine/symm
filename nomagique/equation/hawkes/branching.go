package hawkes

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation/linear"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Parameters are the bivariate exponential Hawkes natural parameters.
*/
type Parameters struct {
	MuX     float64
	MuY     float64
	AlphaXX float64
	AlphaXY float64
	AlphaYX float64
	AlphaYY float64
	Beta    float64
}

/*
BranchingResult is the offspring matrix, spectral radius and stationary means.
Supercritical parameters still report radius; means are undefined (NaN).
*/
type BranchingResult struct {
	SpectralRadius        float64
	StationaryDeterminant float64
	OffspringX            float64
	OffspringY            float64
	Defined               bool
	MeanX                 float64
	MeanY                 float64
	DescendantsX          float64
	DescendantsY          float64
}

/*
Branching owns those formulas.
*/
type Branching struct {
	core.Base[Parameters, BranchingResult]
	finite *logic.Finite[float64]
	radius *linear.SpectralRadius2
}

func NewBranching() *Branching {
	return &Branching{
		finite: logic.NewFinite[float64](),
		radius: linear.NewSpectralRadius2(),
	}
}

func (op *Branching) Next(
	in iter.Seq[core.Primitive[Parameters, Parameters]],
) iter.Seq[core.Primitive[BranchingResult, BranchingResult]] {
	return func(yield func(core.Primitive[BranchingResult, BranchingResult]) bool) {
		for arriving := range in {
			p := arriving.Read()

			if !hawkesFinite(op.finite, p) || p.Beta <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			a, b, c, d := p.AlphaXX/p.Beta, p.AlphaXY/p.Beta, p.AlphaYX/p.Beta, p.AlphaYY/p.Beta
			radius := 0.0

			for out := range op.radius.Next(transport.Values(linear.Matrix2{A: a, B: b, C: c, D: d})) {
				radius = out.Read()
			}

			det := (1-a)*(1-d) - b*c
			result := BranchingResult{
				SpectralRadius:        radius,
				StationaryDeterminant: det,
				OffspringX:            a + c,
				OffspringY:            b + d,
				Defined:               radius < 1,
			}

			if result.Defined {
				result.MeanX = ((1-d)*p.MuX + b*p.MuY) / det
				result.MeanY = (c*p.MuX + (1-a)*p.MuY) / det
				result.DescendantsX = (1-d+c)/det - 1
				result.DescendantsY = (1-a+b)/det - 1
			}

			if !result.Defined {
				result.MeanX = math.NaN()
				result.MeanY = math.NaN()
				result.DescendantsX = math.NaN()
				result.DescendantsY = math.NaN()
			}

			if !yield(op.Carrier(result)) {
				return
			}
		}
	}
}

func hawkesFinite(predicate *logic.Finite[float64], p Parameters) bool {
	for _, value := range []float64{p.MuX, p.MuY, p.AlphaXX, p.AlphaXY, p.AlphaYX, p.AlphaYY, p.Beta} {
		ok := true

		for decision := range predicate.Next(transport.Values(value)) {
			ok = decision.Read()
		}

		if !ok || value < 0 {
			return false
		}
	}

	return true
}
