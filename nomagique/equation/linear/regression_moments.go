package linear

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Point is one scalar (x, y) observation.
*/
type Point struct {
	X float64
	Y float64
}

/*
Moments are the five normal-equation sums and the observation count.
*/
type Moments struct {
	Count float64
	SumX  float64
	SumY  float64
	SumXX float64
	SumXY float64
	SumYY float64
}

/*
RegressionMoments retains those sums. Non-finite coordinates are a domain
failure, not a skipped observation.
*/
type RegressionMoments struct {
	core.Base[Point, Moments]
	moments Moments
	finite  *logic.Finite[float64]
}

func NewRegressionMoments() *RegressionMoments {
	return &RegressionMoments{finite: logic.NewFinite[float64]()}
}

func (op *RegressionMoments) Next(
	in iter.Seq[core.Primitive[Point, Point]],
) iter.Seq[core.Primitive[Moments, Moments]] {
	return func(yield func(core.Primitive[Moments, Moments]) bool) {
		for arriving := range in {
			point := arriving.Read()

			if !defined(op.finite, point.X) || !defined(op.finite, point.Y) {
				op.Error(core.ErrDomain)
				return
			}

			op.moments.Count++
			op.moments.SumX += point.X
			op.moments.SumY += point.Y
			op.moments.SumXX += point.X * point.X
			op.moments.SumXY += point.X * point.Y
			op.moments.SumYY += point.Y * point.Y

			if !yield(op.Carrier(op.moments)) {
				return
			}
		}
	}
}

func defined(predicate *logic.Finite[float64], value float64) bool {
	ok := true

	for decision := range predicate.Next(transport.Values(value)) {
		ok = decision.Read()
	}

	return ok
}
