package arithmetic

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

type Multiply types.Value[[2]float64, float64]

/*
NewMultiply takes a pair of floats and returns their product.
*/
func NewMultiply() Multiply {
	return func(in [2]float64) float64 {
		return in[0] * in[1]
	}
}

type Divide types.Value[[2]float64, float64]

/*
NewDivide takes a pair of floats and returns their quotient.
We do not use fallback defaults or Inf checks; we return pure mathematical results.
*/
func NewDivide() Divide {
	return func(in [2]float64) float64 {
		return in[0] / in[1]
	}
}

type Add types.Value[[2]float64, float64]

/*
NewAdd takes a pair of floats and returns their sum.
*/
func NewAdd() Add {
	return func(in [2]float64) float64 {
		return in[0] + in[1]
	}
}

type Subtract types.Value[[2]float64, float64]

/*
NewSubtract takes a pair of floats and returns their difference (in[0] - in[1]).
*/
func NewSubtract() Subtract {
	return func(in [2]float64) float64 {
		return in[0] - in[1]
	}
}

type SquareRoot types.Value[float64, float64]

/*
NewSquareRoot calculates the principal square root.
*/
func NewSquareRoot() SquareRoot {
	return math.Sqrt
}

type DotProduct types.Value[[2][]float64, float64]

/*
NewDotProduct calculates the dot product of two equal-length float slices.
*/
func NewDotProduct() DotProduct {
	return func(in [2][]float64) float64 {
		a, b := in[0], in[1]
		n := min(len(a), len(b))
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * b[i]
		}
		return sum
	}
}

type Transpose types.Value[[][]float64, [][]float64]

/*
NewTranspose transposes a 2D float matrix.
*/
func NewTranspose() Transpose {
	return func(m [][]float64) [][]float64 {
		if len(m) == 0 {
			return nil
		}
		rows := len(m)
		cols := len(m[0])
		out := make([][]float64, cols)
		for i := range cols {
			out[i] = make([]float64, rows)
			for j := range rows {
				out[i][j] = m[j][i]
			}
		}
		return out
	}
}

type Identity types.Value[int, [][]float64]

/*
NewIdentity returns an n x n identity matrix.
*/
func NewIdentity() Identity {
	return func(n int) [][]float64 {
		m := make([][]float64, n)
		for i := range n {
			m[i] = make([]float64, n)
			m[i][i] = core.Unit
		}
		return m
	}
}

type SpectralRadius2 types.Value[[2][2]float64, float64]

/*
NewSpectralRadius2 calculates the spectral radius of a 2x2 matrix.
Complex eigenvalues use modulus; real eigenvalues use maximum absolute value.
*/
func NewSpectralRadius2() SpectralRadius2 {
	return func(matrix [2][2]float64) float64 {
		trace := matrix[0][0] + matrix[1][1]
		determinant := matrix[0][0]*matrix[1][1] - matrix[0][1]*matrix[1][0]
		discriminant := trace*trace - 4*determinant

		if discriminant < 0 {
			modulus := math.Sqrt(-discriminant)
			realPart := trace / 2
			imagPart := modulus / 2
			return math.Sqrt(realPart*realPart + imagPart*imagPart)
		}

		rootHigh := (trace + math.Sqrt(discriminant)) / 2
		rootLow := (trace - math.Sqrt(discriminant)) / 2

		return math.Max(math.Abs(rootHigh), math.Abs(rootLow))
	}
}

type Sum types.Value[float64, float64]

/*
NewSum creates a stateful accumulator that sums incoming values.
The state is perfectly closed over, requiring zero structs.
*/
func NewSum() Sum {
	var sum float64
	return func(in float64) float64 {
		sum += in
		return sum
	}
}

type Exp types.Value[float64, float64]

/*
NewExp calculates the base-e exponential.
*/
func NewExp() Exp {
	return math.Exp
}

type Clamp types.Value[float64, float64]

/*
NewClamp creates a state-free closure that restricts input to the specified bounds.
*/
func NewClamp(lower, upper float64) Clamp {
	return func(in float64) float64 {
		if in < lower {
			return lower
		}
		if in > upper {
			return upper
		}
		return in
	}
}

