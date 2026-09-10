package correlation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
LagShapeInput is a completed profile and the selected index, not a new search.
*/
type LagShapeInput struct {
	Profile []equation.LagCandidate
	Index   float64
	Span    float64
	Spacing float64
}

/*
LagShapeResult reads the neighbours of the selected index. Undefined or
out-of-range neighbours yield ShapeDefined=false, never a leftover derivative.
Coordinates are seconds.
*/
type LagShapeResult struct {
	ShapeDefined bool
	Prominence   float64
	Curvature    float64
}

/*
LagShape owns that neighbour projection.
*/
type LagShape struct {
	core.Base[LagShapeInput, LagShapeResult]
	difference *equation.SecondDifference[float64]
}

func NewLagShape() *LagShape {
	return &LagShape{difference: equation.NewSecondDifference[float64]()}
}

func (op *LagShape) Next(
	in iter.Seq[core.Primitive[LagShapeInput, LagShapeInput]],
) iter.Seq[core.Primitive[LagShapeResult, LagShapeResult]] {
	return func(yield func(core.Primitive[LagShapeResult, LagShapeResult]) bool) {
		for arriving := range in {
			result, err := op.Evaluate(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(result)) {
				return
			}
		}
	}
}

func (op *LagShape) Evaluate(input LagShapeInput) (LagShapeResult, error) {
	index := int(input.Index)

	if !(input.Index > 0 && input.Index < input.Span*2) {
		return LagShapeResult{}, nil
	}

	if index <= 0 || index >= len(input.Profile)-1 {
		return LagShapeResult{}, nil
	}

	lower := input.Profile[index-1]
	upper := input.Profile[index+1]

	if !lower.Defined() || !upper.Defined() {
		return LagShapeResult{}, nil
	}

	diff, err := transport.Evaluate(op.difference, transport.Values(equation.SecondDifferenceInput[float64]{
		Left:   math.Abs(lower.Y),
		Center: math.Abs(input.Profile[index].Y),
		Right:  math.Abs(upper.Y),
	}))

	if err != nil {
		return LagShapeResult{}, err
	}

	seconds := input.Spacing * 1e-9

	return LagShapeResult{
		ShapeDefined: true,
		Prominence:   diff / 2,
		Curvature:    diff / (seconds * seconds),
	}, nil
}
