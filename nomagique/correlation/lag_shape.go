package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
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
	err error
	out LagShapeResult
}

func NewLagShape() core.Primitive {
	return &LagShape{}
}

func (op *LagShape) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*LagShapeInput)(arriving)
			index := int(input.Index)
			op.out = LagShapeResult{}

			if input.Index > 0 && input.Index < input.Span*2 && index > 0 && index < len(input.Profile)-1 {
				lower := input.Profile[index-1]
				upper := input.Profile[index+1]

				if lower.Defined && upper.Defined {
					leftVal := math.Abs(lower.Y)
					centerVal := math.Abs(input.Profile[index].Y)
					rightVal := math.Abs(upper.Y)
					diff := 2.0*centerVal - leftVal - rightVal
					seconds := input.Spacing * 1e-9

					op.out = LagShapeResult{
						ShapeDefined: true,
						Prominence:   diff / 2.0,
						Curvature:    diff / (seconds * seconds),
					}
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *LagShape) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
