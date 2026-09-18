package arithmetic

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Multiply takes a pair of floats and returns their product.
*/
var Multiply types.Value[[2]float64, float64] = func(in [2]float64) float64 {
	return in[0] * in[1]
}

/*
Divide takes a pair of floats and returns their quotient.
We do not use fallback defaults or Inf checks; we return pure mathematical results.
*/
var Divide types.Value[[2]float64, float64] = func(in [2]float64) float64 {
	return in[0] / in[1]
}

/*
SquareRoot calculates the principal square root.
*/
var SquareRoot types.Value[float64, float64] = math.Sqrt

/*
NewSum creates a stateful accumulator that sums incoming values.
The state is perfectly closed over, requiring zero structs.
*/
func NewSum() types.Value[float64, float64] {
	var sum float64
	return func(in float64) float64 {
		sum += in
		return sum
	}
}
