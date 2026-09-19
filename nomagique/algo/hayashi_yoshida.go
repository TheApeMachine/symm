package algo

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewHayashiYoshida creates a stateful, asynchronous covariance estimator.

Notice that there is ZERO implementation logic here, and ZERO DTOs.
By currying the inputs, we can pass multiple different types (Bounds, then Returns)
through the pure `Value[T, U]` pipeline without ever defining a struct.
*/
type HayashiYoshida types.Value[[2][2]int64, types.Value[[2]float64, float64]]
func NewHayashiYoshida() HayashiYoshida {
	// Instantiate the stateful mathematical accumulators
	covSum := arithmetic.NewSum()
	leftEnergySum := arithmetic.NewSum()
	rightEnergySum := arithmetic.NewSum()

	// 1. First stage of the pipeline takes the time boundaries (Geometry)
	return func(bounds [2][2]int64) types.Value[[2]float64, float64] {
		intersectOp := geometry.NewIntersection()
		isOverlapping := intersectOp(bounds)

		// 2. Second stage takes the returns (Arithmetic)
		return func(returns [2]float64) float64 {
			multOp := arithmetic.NewMultiply()
			var covariance float64
			if isOverlapping {
				// Arithmetic (Covariance product & accumulation)
				covariance = covSum(multOp(returns))
			} else {
				covariance = covSum(0) // Read current sum
			}

			sqrtOp := arithmetic.NewSquareRoot()
			scale := sqrtOp(
				multOp([2]float64{
					leftEnergySum(multOp([2]float64{returns[0], returns[0]})),
					rightEnergySum(multOp([2]float64{returns[1], returns[1]})),
				}),
			)

			divOp := arithmetic.NewDivide()
			if scale == 0 {
				return 0
			}
			return divOp([2]float64{covariance, scale})
		}
	}
}
