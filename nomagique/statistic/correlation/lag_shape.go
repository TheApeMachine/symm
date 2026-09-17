package correlation

import (
	"iter"
	"math"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
LagShapeInput is a completed profile and the selected index, not a new search.
*/
type LagShapeInput struct {
	Profile []LagCandidate
	Index   float64
	Span    float64
	Spacing float64
}

/*
LagShapeResult reads the neighbours of the selected index. Undefined or
out-of-range neighbours yield ShapeDefined=false.
*/
type LagShapeResult struct {
	ShapeDefined bool
	Prominence   float64
	Curvature    float64
}

/*
LagShape owns neighbour projection around the selected candidate.
*/
type LagShape struct {
	*core.PrimitiveError

	diff core.Primitive
}

func NewLagShape() *LagShape {
	return &LagShape{
		PrimitiveError: core.NewPrimitiveError(),
		diff:           calculus.NewSecondDifference(),
	}
}

func (lagShape *LagShape) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*LagShapeInput)(arriving)
			index := int(input.Index)
			result := LagShapeResult{}

			if input.Index > 0 && input.Index < input.Span+input.Span && index > 0 && index < len(input.Profile)-1 {
				lower := input.Profile[index-1]
				upper := input.Profile[index+1]

				if lower.Defined && upper.Defined {
					leftVal := math.Abs(lower.Y)
					centerVal := math.Abs(input.Profile[index].Y)
					rightVal := math.Abs(upper.Y)

					diffRecord := calculus.SecondDifferenceInput{
						Left:   leftVal,
						Center: centerVal,
						Right:  rightVal,
					}
					var diff float64

					for out := range lagShape.diff.Next(sequence.NewOne(unsafe.Pointer(&diffRecord)).Next(nil)) {
						diff = *(*float64)(out)
					}

					seconds := input.Spacing / float64(time.Second)

					result = LagShapeResult{
						ShapeDefined: true,
						Prominence:   diff / (core.Unit + core.Unit),
						Curvature:    diff / (seconds * seconds),
					}
				}
			}

			if !yield(unsafe.Pointer(&result)) {
				return
			}
		}
	}
}
